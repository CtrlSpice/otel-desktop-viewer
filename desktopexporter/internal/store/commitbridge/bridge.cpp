#include "duckdb/main/client_context.hpp"
#include "duckdb/main/client_context_state.hpp"
#include "duckdb/main/config.hpp"
#include "duckdb/main/database.hpp"
#include "duckdb/main/extension/extension_loader.hpp"
#include "duckdb/transaction/meta_transaction.hpp"
#include "duckdb_static_extension.h"

extern "C" void otel_commit_notify(uint64_t id);

namespace {
using namespace duckdb;

class CommitObserver final : public ClientContextState {
public:
    explicit CommitObserver(uint64_t id) : id(id) {}

    void TransactionCommit(MetaTransaction &transaction, ClientContext &) override {
        if (transaction.ModifiedDatabase()) {
            otel_commit_notify(id);
        }
    }

private:
    uint64_t id;
};

void SetObserver(ClientContext &context, SetScope scope, Value &value) {
    if (scope != SetScope::SESSION) {
        throw InvalidInputException("Commit observers must be session-scoped");
    }
    context.registered_state->Remove("otel_commits");
    context.registered_state->Insert("otel_commits", make_shared_ptr<CommitObserver>(value.GetValue<uint64_t>()));
}

void Load(ExtensionLoader &loader) {
    // This bridge uses C++ layouts. Refuse a different engine before accessing
    // them, even if its public version number is the same.
    if (string(DuckDB::SourceID()) != "d54cbbe3df") {
        throw InvalidInputException("otel_commits requires DuckDB source d54cbbe3df");
    }
    auto &config = DBConfig::GetConfig(loader.GetDatabaseInstance());
    config.AddExtensionOption("otel_commit_observer", "Viewer commit observer", LogicalType::UBIGINT,
                              Value::UBIGINT(0), SetObserver);
}

int32_t Describe(duckdb_extension_descriptor *descriptor) {
    descriptor->name = "otel_commits";
    descriptor->extension_version = "1";
    descriptor->api_version = "v2.0.0";
    descriptor->entry_cpp = reinterpret_cast<void (*)(void)>(Load);
    return 0;
}
} // namespace

extern "C" int32_t otel_commits_register(void) {
    return duckdb_register_static_extension(Describe);
}
