#include "bridge.h"
#include "include/duckdb_static_extension.h"
#include <string.h>

#ifdef _WIN32
#include <windows.h>
static SRWLOCK registry_mutex = SRWLOCK_INIT;
#define LOCK() AcquireSRWLockExclusive(&registry_mutex)
#define UNLOCK() ReleaseSRWLockExclusive(&registry_mutex)
#else
#include <pthread.h>
static pthread_mutex_t registry_mutex = PTHREAD_MUTEX_INITIALIZER;
#define LOCK() pthread_mutex_lock(&registry_mutex)
#define UNLOCK() pthread_mutex_unlock(&registry_mutex)
#endif

#define CHECK(call) do { if ((call) != DUCKDB_V2_ERROR_NONE) goto cleanup; } while (0)

// Only outstanding synchronous inserts are registered. SQL receives a token,
// never an address. Bind data keeps the token so a stale prepared plan cannot
// dereference an input after its caller has released it.
struct input {
    uint64_t token;
    struct ArrowSchema *schema;
    struct ArrowArray *array;
    struct input *next;
};
static struct input *inputs;
static uint64_t next_token;

static struct input *find_input(uint64_t token) {
    for (struct input *p = inputs; p; p = p->next) {
        if (p->token == token) return p;
    }
    return NULL;
}

uint64_t otel_arrow_publish(struct ArrowSchema *schema, struct ArrowArray *array) {
    struct input *p = malloc(sizeof(*p));
    if (!p) return 0;
    LOCK();
    // Do not reuse a token even after wraparound.
    if (next_token == UINT64_MAX) { UNLOCK(); free(p); return 0; }
    p->token = ++next_token;
    p->schema = schema;
    p->array = array;
    p->next = inputs;
    inputs = p;
    uint64_t token = p->token;
    UNLOCK();
    return token;
}

void otel_arrow_unpublish(uint64_t token) {
    LOCK();
    struct input **link = &inputs;
    while (*link && (*link)->token != token) link = &(*link)->next;
    struct input *p = *link;
    if (p) *link = p->next;
    UNLOCK();
    if (!p) return;
    // The importer clears release when it takes ownership. Otherwise the query
    // failed before consuming the batch, and the caller releases it here.
    if (p->array->release) p->array->release(p->array);
    if (p->schema->release) p->schema->release(p->schema);
    free(p);
}

static void fail(duckdb_v2_error_info_handle *err, const char *message) {
    duckdb_v2_str text = {message, strlen(message)};
    duckdb_v2_error_info_set_code(*err, DUCKDB_V2_ERROR_INPUT_INVALID);
    duckdb_v2_error_info_set_text(*err, &text);
}

struct bound { uint64_t token; };
static void destroy_bound(void *data) { free(data); }
struct scan { duckdb_v2_arrow_importer_handle importer; };
static void destroy_scan(void *data) {
    struct scan *state = data;
    duckdb_v2_arrow_importer_destroy(&state->importer);
    free(state);
}

static void bind_input(duckdb_v2_function_bind_info_handle info,
                       duckdb_v2_table_function_bind_info_handle result,
                       duckdb_v2_context_handle context, duckdb_v2_error_info_handle *err) {
    duckdb_v2_value_handle value = NULL;
    duckdb_v2_arrow_importer_handle importer = NULL;
    duckdb_v2_schema_handle schema = NULL;
    struct bound *bound = NULL;
    uint64_t token;
    bool locked = false;
    CHECK(duckdb_v2_function_bind_get_arg_value(info, 0, &value, err));
    CHECK(duckdb_v2_value_get_ubigint(value, &token, err));
    LOCK(); locked = true;
    struct input *input = find_input(token);
    if (!input) { fail(err, "expired Arrow batch token"); goto cleanup; }
    CHECK(duckdb_v2_arrow_importer_create(context, input->schema, 2048, &importer, err));
    CHECK(duckdb_v2_arrow_importer_get_schema(importer, &schema, err));
    idx_t count;
    CHECK(duckdb_v2_schema_get_count(schema, &count, err));
    for (idx_t i = 0; i < count; i++) {
        duckdb_v2_identifier_t name;
        duckdb_v2_logical_type_handle type;
        // Both name and type are borrowed from schema.
        CHECK(duckdb_v2_schema_get_field(schema, i, &name, &type, err));
        CHECK(duckdb_v2_table_function_bind_add_result_column(result, &name, type, err));
    }
    bound = calloc(1, sizeof(*bound));
    if (!bound) { fail(err, "Arrow bind allocation failed"); goto cleanup; }
    bound->token = token;
    duckdb_v2_opaque data = {bound, destroy_bound, NULL};
    CHECK(duckdb_v2_function_bind_set_bind_data(info, &data, err));
    bound = NULL;
cleanup:
    if (locked) UNLOCK();
    free(bound);
    duckdb_v2_schema_destroy(&schema);
    duckdb_v2_arrow_importer_destroy(&importer);
    duckdb_v2_value_destroy(&value);
}

static void init_scan(duckdb_v2_table_function_init_global_info_handle info,
                      duckdb_v2_context_handle context, duckdb_v2_error_info_handle *err) {
    struct bound *bound = NULL;
    struct scan *scan = NULL;
    bool locked = false;
    CHECK(duckdb_v2_table_function_init_global_get_bind_data(info, (void **)&bound, err));
    scan = calloc(1, sizeof(*scan));
    if (!scan) { fail(err, "Arrow scan allocation failed"); goto cleanup; }
    LOCK(); locked = true;
    struct input *input = find_input(bound->token);
    if (!input || !input->array->release) { fail(err, "Arrow batch expired or already consumed"); goto cleanup; }
    CHECK(duckdb_v2_arrow_importer_create(context, input->schema, 2048, &scan->importer, err));
    CHECK(duckdb_v2_arrow_importer_append(scan->importer, input->array, true, true, err));
    CHECK(duckdb_v2_table_function_init_global_set_max_threads(info, 1, err));
    duckdb_v2_opaque data = {scan, destroy_scan, NULL};
    CHECK(duckdb_v2_table_function_init_global_set_global_state(info, &data, err));
    scan = NULL;
cleanup:
    if (locked) UNLOCK();
    if (scan) destroy_scan(scan);
}

static void execute_scan(duckdb_v2_table_function_exec_info_handle info,
                         duckdb_v2_context_handle context, duckdb_v2_error_info_handle *err) {
    (void)context;
    struct scan *scan;
    duckdb_v2_data_chunk_handle chunk = NULL, output = NULL;
    CHECK(duckdb_v2_table_function_exec_get_global_state(info, (void **)&scan, err));
    CHECK(duckdb_v2_arrow_importer_next_chunk(scan->importer, &chunk, err));
    if (!chunk) goto cleanup;
    CHECK(duckdb_v2_table_function_exec_get_output_chunk(info, &output, err));
    idx_t count;
    CHECK(duckdb_v2_data_chunk_get_vector_count(chunk, &count, err));
    for (idx_t i = 0; i < count; i++) {
        duckdb_v2_vector_handle source, target;
        CHECK(duckdb_v2_data_chunk_get_vector(chunk, i, &source, err));
        CHECK(duckdb_v2_data_chunk_get_vector(output, i, &target, err));
        CHECK(duckdb_v2_vector_reference(target, source, err));
    }
cleanup:
    duckdb_v2_data_chunk_destroy(&chunk);
}

static void initialize(duckdb_v2_extension_input *input) {
    duckdb_v2_extension_handle extension = input->extension;
    duckdb_v2_context_handle context = input->context;
    duckdb_v2_error_info_handle *err = input->err;
    duckdb_v2_factory_handle factory = NULL;
    duckdb_v2_table_function_handle function = NULL;
    duckdb_v2_logical_type_handle type = NULL;
    duckdb_v2_function_signature_handle signature = NULL;
    duckdb_v2_str name = {"otel_arrow_scan", 15}, arg = {"batch", 5};
    CHECK(duckdb_v2_context_get_factory(context, &factory, err));
    CHECK(duckdb_v2_table_function_create(factory, &function, err));
    CHECK(duckdb_v2_table_function_set_name(function, &name, err));
    CHECK(duckdb_v2_factory_create_type_from_id(factory, DUCKDB_V2_LOGICAL_TYPE_ID_UBIGINT, NULL, NULL, 0, &type, err));
    CHECK(duckdb_v2_table_function_get_signature(function, &signature, err));
    CHECK(duckdb_v2_function_signature_add_parameter(signature, &arg, type, NULL, DUCKDB_V2_FUNCTION_PARAMETER_KIND_POSITIONAL_ONLY, err));
    CHECK(duckdb_v2_table_function_set_bind_callback(function, bind_input, err));
    CHECK(duckdb_v2_table_function_set_init_global_callback(function, init_scan, err));
    CHECK(duckdb_v2_table_function_set_exec_callback(function, execute_scan, err));
    CHECK(duckdb_v2_extension_register_table_function(extension, function, err));
    CHECK(otel_arrow_register_output(extension, factory, err));
cleanup:
    duckdb_v2_logical_type_destroy(&type);
    duckdb_v2_table_function_destroy(&function);
}

static int32_t describe(duckdb_extension_descriptor *descriptor) {
    if (descriptor->version < 1) return 1;
    descriptor->version = 1;
    descriptor->name = "otel_arrow";
    descriptor->extension_version = "1.0.0";
    descriptor->api_version = "v2.0.0";
    descriptor->entry_capi_v2 = (void (*)(void))initialize;
    return 0;
}

int otel_arrow_register(void) { return duckdb_register_static_extension(describe); }
