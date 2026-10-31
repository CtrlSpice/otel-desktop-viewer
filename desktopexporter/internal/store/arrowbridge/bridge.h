#pragma once
#define DUCKDB_V2_API_ALLOW_UNSTABLE 1
#include "include/duckdb_v2.h"
#include <stdlib.h>

int otel_arrow_register(void);
uint64_t otel_arrow_publish(struct ArrowSchema *schema, struct ArrowArray *array);
void otel_arrow_unpublish(uint64_t token);
DUCKDB_V2_ERROR otel_arrow_register_output(duckdb_v2_extension_handle extension,
    duckdb_v2_factory_handle factory, duckdb_v2_error_info_handle *err);
