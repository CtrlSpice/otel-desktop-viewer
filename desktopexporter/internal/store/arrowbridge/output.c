#include "bridge.h"
#include <stdio.h>
#include <string.h>

// Go consumes the schema/array synchronously. A token identifies a live call,
// never a pointer supplied through SQL. No filesystem is involved in this COPY
// format. DuckDB serializes flush callbacks in input order for this one output.
extern int otelArrowOutput(uint64_t token, struct ArrowSchema *schema, struct ArrowArray *array);

#define CHECK(call) do { if ((call) != DUCKDB_V2_ERROR_NONE) goto cleanup; } while (0)

struct output {
    uint64_t token;
    idx_t count;
    duckdb_v2_logical_type_handle *types;
    duckdb_v2_arrow_exporter_handle exporter;
};

static void destroy_output(void *ptr) {
    struct output *out = ptr;
    duckdb_v2_arrow_exporter_destroy(&out->exporter);
    for (idx_t i = 0; i < out->count; i++) duckdb_v2_logical_type_destroy(&out->types[i]);
    free(out->types);
    free(out);
}

static void output_error(duckdb_v2_error_info_handle *err, const char *message) {
    duckdb_v2_str text = {message, strlen(message)};
    duckdb_v2_error_info_set_code(*err, DUCKDB_V2_ERROR_INPUT_INVALID);
    duckdb_v2_error_info_set_text(*err, &text);
}

static void bind_output(duckdb_v2_copy_to_bind_info_handle info,
                        duckdb_v2_context_handle context, duckdb_v2_error_info_handle *err) {
    struct output *out = calloc(1, sizeof(*out));
    duckdb_v2_str *names = NULL;
    duckdb_v2_str path;
    if (!out) { output_error(err, "Arrow output allocation failed"); return; }
    CHECK(duckdb_v2_copy_to_bind_get_file_path(info, &path, err));
    // Parse the exact internal token spelling, without reading beyond the view.
    const char prefix[] = "otel-arrow-";
    if (path.len <= sizeof(prefix)-1 || path.len > sizeof(prefix)+19 ||
        memcmp(path.ptr, prefix, sizeof(prefix)-1)) {
        output_error(err, "invalid Arrow output token"); goto cleanup;
    }
    for (idx_t i = sizeof(prefix)-1; i < path.len; i++) {
        unsigned char c = path.ptr[i];
        if (c < '0' || c > '9' || out->token > (UINT64_MAX-(c-'0'))/10) {
            output_error(err, "invalid Arrow output token"); goto cleanup;
        }
        out->token = out->token*10 + c-'0';
    }
    CHECK(duckdb_v2_copy_to_bind_get_column_count(info, &out->count, err));
    out->types = calloc(out->count, sizeof(*out->types));
    names = calloc(out->count, sizeof(*names));
    if (!out->types || !names) {
        // No type handles exist yet if allocation failed.
        out->count = 0;
        output_error(err, "Arrow output schema allocation failed"); goto cleanup;
    }
    for (idx_t i = 0; i < out->count; i++) {
        CHECK(duckdb_v2_copy_to_bind_get_column_type(info, i, &out->types[i], err));
        CHECK(duckdb_v2_copy_to_bind_get_column_name(info, i, &names[i], err));
    }
    CHECK(duckdb_v2_arrow_exporter_create(context, out->types, names, out->count, 0, &out->exporter, err));
    duckdb_v2_opaque data = {out, destroy_output, NULL};
    CHECK(duckdb_v2_copy_to_bind_set_bind_data(info, &data, err));
    out = NULL;
cleanup:
    free(names);
    if (out) destroy_output(out);
}

static void init_output(duckdb_v2_copy_to_init_info_handle info,
                        duckdb_v2_context_handle context, duckdb_v2_error_info_handle *err) {
    (void)context;
    struct output *out;
    struct ArrowSchema schema = {0};
    CHECK(duckdb_v2_copy_to_init_get_bind_data(info, (void **)&out, err));
    CHECK(duckdb_v2_arrow_exporter_get_schema(out->exporter, &schema, err));
    if (otelArrowOutput(out->token, &schema, NULL)) output_error(err, "Arrow output consumer failed");
cleanup:
    if (schema.release) schema.release(&schema);
}

static void batch_size(duckdb_v2_copy_to_batch_size_info_handle info,
                       duckdb_v2_context_handle context, duckdb_v2_error_info_handle *err) {
    (void)context;
    duckdb_v2_copy_to_batch_size_set_target(info, 8192, err);
}

static void destroy_batch(void *ptr) {
    duckdb_v2_column_data_collection_handle collection = ptr;
    duckdb_v2_column_data_collection_destroy(&collection);
}

static void batch_output(duckdb_v2_copy_to_batch_info_handle info,
                         duckdb_v2_context_handle context, duckdb_v2_error_info_handle *err) {
    (void)context;
    duckdb_v2_column_data_collection_handle collection = NULL;
    CHECK(duckdb_v2_copy_to_batch_take_input(info, &collection, err));
    duckdb_v2_opaque data = {collection, destroy_batch, NULL};
    CHECK(duckdb_v2_copy_to_batch_set_batch_data(info, &data, err));
    collection = NULL;
cleanup:
    duckdb_v2_column_data_collection_destroy(&collection);
}

static void flush_output(duckdb_v2_copy_to_flush_info_handle info,
                         duckdb_v2_context_handle context, duckdb_v2_error_info_handle *err) {
    struct output *out;
    duckdb_v2_column_data_collection_handle collection;
    duckdb_v2_column_data_collection_shared_scan_state_handle shared = NULL;
    duckdb_v2_column_data_collection_worker_scan_state_handle worker = NULL;
    duckdb_v2_data_chunk_handle chunk = NULL;
    duckdb_v2_factory_handle factory;
    struct ArrowArray array = {0};
    CHECK(duckdb_v2_copy_to_flush_get_bind_data(info, (void **)&out, err));
    CHECK(duckdb_v2_copy_to_flush_get_batch_data(info, (void **)&collection, err));
    CHECK(duckdb_v2_context_get_factory(context, &factory, err));
    CHECK(duckdb_v2_data_chunk_create(factory, out->types, out->count, &chunk, err));
    CHECK(duckdb_v2_column_data_collection_shared_scan_state_create(collection, &shared, err));
    CHECK(duckdb_v2_column_data_collection_worker_scan_state_create(collection, &worker, err));
    for (;;) {
        bool produced;
        CHECK(duckdb_v2_column_data_collection_scan(collection, shared, worker, chunk, &produced, err));
        if (!produced) break;
        CHECK(duckdb_v2_arrow_exporter_append(out->exporter, &chunk, false, false, err));
        for (;;) {
            CHECK(duckdb_v2_arrow_exporter_next_array(out->exporter, &array, err));
            if (!array.release) break;
            if (otelArrowOutput(out->token, NULL, &array)) {
                output_error(err, "Arrow output consumer failed"); goto cleanup;
            }
            if (array.release) array.release(&array);
        }
    }
cleanup:
    if (array.release) array.release(&array);
    duckdb_v2_data_chunk_destroy(&chunk);
    duckdb_v2_column_data_collection_worker_scan_state_destroy(&worker);
    duckdb_v2_column_data_collection_shared_scan_state_destroy(&shared);
}

DUCKDB_V2_ERROR otel_arrow_register_output(duckdb_v2_extension_handle extension,
    duckdb_v2_factory_handle factory, duckdb_v2_error_info_handle *err) {
    duckdb_v2_copy_function_handle function = NULL;
    DUCKDB_V2_ERROR result = DUCKDB_V2_ERROR_INPUT_INVALID;
    duckdb_v2_str name = {"otel_arrow_output", 17};
    CHECK(duckdb_v2_copy_function_create(factory, &function, err));
    CHECK(duckdb_v2_copy_function_set_name(function, &name, err));
    CHECK(duckdb_v2_copy_to_set_bind_callback(function, bind_output, err));
    CHECK(duckdb_v2_copy_to_set_init_callback(function, init_output, err));
    CHECK(duckdb_v2_copy_to_set_batch_size_callback(function, batch_size, err));
    CHECK(duckdb_v2_copy_to_set_batch_callback(function, batch_output, err));
    CHECK(duckdb_v2_copy_to_set_flush_callback(function, flush_output, err));
    result = duckdb_v2_extension_register_copy_function(extension, function, err);
cleanup:
    duckdb_v2_copy_function_destroy(&function);
    return result;
}
