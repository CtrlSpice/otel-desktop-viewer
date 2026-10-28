#pragma once
#define DUCKDB_V2_API_ALLOW_UNSTABLE 1
#include "include/duckdb_v2.h"
#include <stdlib.h>

int otel_arrow_register(void);
uint64_t otel_arrow_publish(struct ArrowSchema *schema, struct ArrowArray *array);
void otel_arrow_unpublish(uint64_t token);
