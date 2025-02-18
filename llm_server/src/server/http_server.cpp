#include "server/http_server.hpp"
#include "server/request_handler.hpp"
#include "utils/logger.hpp"
#include "utils/multipart_utils.hpp"

#include <crow.h>
#include <crow/multipart.h>
#include <nlohmann/json.hpp>

#ifdef __linux__
#include <unistd.h>
#include <fstream>
#endif

#include <chrono>
#include <ctime>
#include <atomic>
#include <sstream>

namespace {
    // Server start time
    static auto g_serverStartTime = std::chrono::steady_clock::now();

    // Total requests (for metrics)
    static std::atomic<long> g_totalRequests{0};

#ifdef __linux__
    static long getMemoryUsageKB()
    {
        std::ifstream statm("/proc/self/statm");
        long totalPages = 0, residentPages = 0, share = 0;
        if (statm.good()) {
            statm >> totalPages >> residentPages >> share;
            long pageSizeKB = sysconf(_SC_PAGE_SIZE) / 1024;
            return residentPages * pageSizeKB;
        }
        return -1;
    }

    static std::string formatMemorySizeBytes(long bytes)
    {
        if (bytes < 0) {
            return "unknown";
        }
        static const char* SUFFIXES[] = {"B", "KB", "MB", "GB", "TB"};
        int suffixIndex = 0;
        double value = static_cast<double>(bytes);

        while (value >= 1024.0 && suffixIndex < 4) {
            value /= 1024.0;
            ++suffixIndex;
        }
        char buf[64];
        std::snprintf(buf, sizeof(buf), "%.2f %s", value, SUFFIXES[suffixIndex]);
        return std::string(buf);
    }
#endif

    /**
     * Helper to build a JSON response with code=200 by default
     */
    static crow::response makeJSONResponse(const nlohmann::json& j, int code = 200)
    {
        crow::response resp;
        resp.code = code;
        resp.set_header("Content-Type", "application/json");
        resp.body = j.dump();
        return resp;
    }

} // anonymous namespace

namespace server {

void startServer() {
    crow::SimpleApp app;

    // Optional: set a custom log file name
    utils::Logger::setLogFile("/var/log/llm_server/server.log");

    // -----------------------------
    // POST /api/v1/llm/completions
    // -----------------------------
    CROW_ROUTE(app, "/api/v1/llm/completions")
        .methods(crow::HTTPMethod::POST)
    ([&](const crow::request& req){
        g_totalRequests.fetch_add(1, std::memory_order_relaxed);

        utils::Logger::info("[/api/v1/llm/completions] Received request.");

        std::vector<crow::multipart::part> fileParts;
        nlohmann::json bodyJson;
        bool jsonParsed = false;

        auto contentType = req.get_header_value("Content-Type");
        utils::Logger::info("[/api/v1/llm/completions] Content-Type: " + contentType);

        // Attempt parse as multipart
        crow::multipart::message multipartReq(req);
        if (!multipartReq.parts.empty()) {
            for (auto& field : multipartReq.parts) {
                std::string fieldName = utils::getPartName(field);
                if (fieldName == "files") {
                    fileParts.push_back(field);
                } else if (fieldName == "json") {
                    try {
                        bodyJson = nlohmann::json::parse(field.body);
                        jsonParsed = true;
                    } catch(...) {
                        utils::Logger::warn("[/api/v1/llm/completions] Invalid JSON in 'json' part.");
                        nlohmann::json errJson = {
                            {"error_code", 400},
                            {"details", "Invalid JSON in 'json' part"}
                        };
                        return makeJSONResponse(errJson, 400);
                    }
                }
            }
        }

        // If no JSON yet, parse raw request body
        if (!jsonParsed) {
            try {
                bodyJson = nlohmann::json::parse(req.body);
            } catch(...) {
                utils::Logger::warn("[/api/v1/llm/completions] Invalid JSON in request body.");
                nlohmann::json errJson = {
                    {"error_code", 400},
                    {"details", "Invalid JSON in request body"}
                };
                return makeJSONResponse(errJson, 400);
            }
        }

        // Call the actual request handler
        auto resultJson = server::handleLLMQuery(bodyJson, fileParts);

        // The request_handler uses "ecode" for error code. 
        int httpCode = resultJson.value("ecode", 500);

        utils::Logger::info("[/api/v1/llm/completions] Responding with HTTP " + std::to_string(httpCode));
        return makeJSONResponse(resultJson, httpCode);
    });

    // ---------------
    // GET /api/v1/health
    // ---------------
    // Example health endpoint with consistent naming & response
    CROW_ROUTE(app, "/api/v1/health")
    ([](){
        utils::Logger::info("[/api/v1/health] Received request.");

        nlohmann::json health;
        health["health_status"] = "healthy";

        auto now = std::chrono::steady_clock::now();
        auto uptimeSec = std::chrono::duration_cast<std::chrono::seconds>(now - g_serverStartTime).count();
        health["uptime_seconds"] = uptimeSec;

#ifdef __linux__
        long rssKB = getMemoryUsageKB();
        if (rssKB >= 0) {
            long bytes = rssKB * 1024;
            health["memory_usage"] = formatMemorySizeBytes(bytes);
        } else {
            health["memory_usage"] = "unknown";
        }
#else
        health["memory_usage"] = "not available";
#endif
        health["build_version"] = "v1.0.0";

        // UTC time
        auto t = std::time(nullptr);
        std::tm utcTm{};
#ifdef _WIN32
        gmtime_s(&utcTm, &t);
#else
        gmtime_r(&t, &utcTm);
#endif
        char timeBuf[64];
        std::strftime(timeBuf, sizeof(timeBuf), "%Y-%m-%dT%H:%M:%SZ", &utcTm);
        health["timestamp_utc"] = timeBuf;

        // Return standard JSON response
        utils::Logger::info("[/api/v1/health] Responding with HTTP 200");
        return makeJSONResponse(health, 200);
    });

    // ---------------
    // GET /metrics
    // ---------------
    // Simple Prometheus text-based output
    CROW_ROUTE(app, "/metrics")
    ([](){
        utils::Logger::info("[/metrics] Received request.");

        std::ostringstream oss;
        oss << "# HELP llm_server_requests_total The total number of LLM requests processed.\n"
            << "# TYPE llm_server_requests_total counter\n"
            << "llm_server_requests_total " << g_totalRequests.load(std::memory_order_relaxed) << "\n";

        crow::response r;
        r.code = 200;
        r.set_header("Content-Type", "text/plain; version=0.0.4"); 
        r.body = oss.str();

        utils::Logger::info("[/metrics] Responding with HTTP 200");
        return r;
    });

    // ---------------
    // GET /api/v1/logs
    // ---------------
    // Expose recent logs from memory
    CROW_ROUTE(app, "/api/v1/logs")
    ([&](const crow::request& req){
        utils::Logger::info("[/api/v1/logs] Received request.");

        int amount = 50; // default
        if (auto amountStr = req.url_params.get("amount")) {
            try {
                amount = std::stoi(amountStr);
            } catch(...) {
                utils::Logger::warn("[/api/v1/logs] Invalid amount param. Defaulting to 50.");
            }
        }

        auto recentLogs = utils::Logger::getRecentLogs(amount);

        // Build JSON array
        nlohmann::json logsJson = nlohmann::json::array();
        for (auto &logLine : recentLogs) {
            logsJson.push_back(logLine);
        }

        utils::Logger::info("[/api/v1/logs] Responding with last " + std::to_string(amount) + " logs.");
        return makeJSONResponse(logsJson, 200);
    });

    // Start server
    utils::Logger::info("Starting server on port 8080...");
    app.port(8080).multithreaded().run();
}

} // namespace server
