#include "server/http_server.hpp"
#include "server/discord_request.hpp"
#include "utils/logger.hpp"
#include "utils/token_tracker.hpp"

#include <drogon/drogon.h>
#include <drogon/MultiPart.h>
#include <nlohmann/json.hpp>
#include <atomic>
#include <chrono>
#include <ctime>
#include <sstream>

#ifdef __linux__
#include <unistd.h>
#include <fstream>
#endif

namespace
{
    /**
     * @brief The time point at which the server is started.
     */
    static auto g_serverStartTime = std::chrono::steady_clock::now();

    /**
     * @brief Atomic counter to track the total number of requests.
     */
    static std::atomic<long> g_totalRequests{0};

#ifdef __linux__
    /**
     * @brief Retrieves the memory usage in kilobytes (KB) for the current process on Linux.
     *
     * @return The memory usage in KB, or -1 if it could not be determined.
     */
    static long getMemoryUsageKB()
    {
        std::ifstream statm("/proc/self/statm");
        long totalPages = 0;
        long residentPages = 0;
        long share = 0;
        if (statm.good())
        {
            statm >> totalPages >> residentPages >> share;
            long pageSizeKB = sysconf(_SC_PAGE_SIZE) / 1024;
            return residentPages * pageSizeKB;
        }
        return -1;
    }

    /**
     * @brief Formats a size in bytes into a human-readable string (e.g., "12.34 MB").
     *
     * @param bytes The size in bytes.
     * @return The formatted string with the appropriate unit (B, KB, MB, GB, TB).
     */
    static std::string formatMemorySizeBytes(long bytes)
    {
        if (bytes < 0)
        {
            return "unknown";
        }
        static const char* SUFFIXES[] = {"B", "KB", "MB", "GB", "TB"};
        int suffixIndex = 0;
        double value = static_cast<double>(bytes);

        while (value >= 1024.0 && suffixIndex < 4)
        {
            value /= 1024.0;
            ++suffixIndex;
        }
        char buf[64];
        std::snprintf(buf, sizeof(buf), "%.2f %s", value, SUFFIXES[suffixIndex]);
        return std::string(buf);
    }
#endif

    /**
     * @brief Builds and returns a JSON-based HTTP response.
     *
     * @param j    The JSON object to return as the response body.
     * @param code The HTTP status code to set. Defaults to 200.
     * @return A drogon::HttpResponsePtr with the provided JSON body.
     */
    drogon::HttpResponsePtr makeJSONResponse(const nlohmann::json& j, int code = 200)
    {
        auto resp = drogon::HttpResponse::newHttpResponse();
        resp->setStatusCode(static_cast<drogon::HttpStatusCode>(code));
        resp->setContentTypeCode(drogon::CT_APPLICATION_JSON);
        resp->setBody(j.dump());
        return resp;
    }

} // anonymous namespace

namespace server
{

/**
 * @brief Initializes logging, registers all HTTP handlers (endpoints),
 *        and starts the Drogon server.
 *
 * This function configures the logger, sets up the server routes for:
 * - LLM completions (POST /api/v1/chat/completions)
 * - status checks (GET /api/v1/status)
 * - Prometheus-style metrics (GET /metrics)
 * - Log retrieval (GET /api/v1/logs)
 *
 * Finally, it binds the server to 0.0.0.0:8080 and runs the main loop.
 */
void startServer()
{
    // Configure logging
    utils::Logger::setLogFile("/var/log/llm_server/server.log");
    utils::Logger::info("Logger initialized and file set to /var/log/llm_server/server.log.");

    // ----------------------------------------------------------------------
    // POST /api/v1/chat/completions
    // ----------------------------------------------------------------------
    drogon::app().registerHandler(
        "/api/v1/chat/completions",
        [](const drogon::HttpRequestPtr& req,
           std::function<void(const drogon::HttpResponsePtr&)>&& callback)
        {
            utils::Logger::info("[/api/v1/chat/completions] Received request.");

            nlohmann::json bodyJson;
            std::vector<utils::MultipartPart> fileParts;
            bool jsonParsed = false;

            // Handle multipart form data if present
            if (req->contentType() == drogon::CT_MULTIPART_FORM_DATA)
            {
                drogon::MultiPartParser parser;
                if (parser.parse(req) == 0)
                {
                    auto jsonField = parser.getParameter<std::string>("json");
                    if (!jsonField.empty())
                    {
                        try
                        {
                            bodyJson = nlohmann::json::parse(jsonField);
                            jsonParsed = true;
                            utils::Logger::info("[/api/v1/chat/completions] JSON parsed from multipart form data.");
                        }
                        catch (...)
                        {
                            utils::Logger::error("[/api/v1/chat/completions] Invalid JSON in 'json' field.");
                            nlohmann::json errJson{
                                {"error_code", 400},
                                {"details", "Invalid JSON in 'json' field"}};
                            callback(makeJSONResponse(errJson, 400));
                            return;
                        }
                    }

                    // Collect any uploaded files
                    auto uploadedFiles = parser.getFiles();
                    for (auto& f : uploadedFiles)
                    {
                        utils::MultipartPart part;
                        part.filename = f.getFileName();
                        part.contentType = f.getContentType();
                        part.body = std::string{f.fileContent()};
                        fileParts.push_back(std::move(part));
                    }

                    if (!uploadedFiles.empty())
                    {
                        utils::Logger::info("[/api/v1/chat/completions] Received "
                                            + std::to_string(uploadedFiles.size()) + " file(s).");
                    }
                }
                else
                {
                    utils::Logger::warn("[/api/v1/chat/completions] Failed to parse multipart data.");
                    nlohmann::json errJson{
                        {"error_code", 400},
                        {"details", "Failed to parse multipart data"}};
                    callback(makeJSONResponse(errJson, 400));
                    return;
                }
            }

            // If JSON was not parsed yet, attempt to parse the request body as JSON
            if (!jsonParsed && !req->body().empty())
            {
                try
                {
                    bodyJson = nlohmann::json::parse(req->body());
                    utils::Logger::info("[/api/v1/chat/completions] JSON parsed from request body.");
                }
                catch (...)
                {
                    utils::Logger::error("[/api/v1/chat/completions] Invalid JSON in body.");
                    nlohmann::json errJson{
                        {"error_code", 400},
                        {"details", "Invalid JSON in body"}};
                    callback(makeJSONResponse(errJson, 400));
                    return;
                }
            }

            // Log request body size
            utils::Logger::info("[/api/v1/chat/completions] Request body size: "
                                + std::to_string(req->body().size()) + " bytes.");

            // ------------------------------------------------------------------
            // Check the "purpose" parameter
            // ------------------------------------------------------------------
            if (!bodyJson.contains("purpose"))
            {
                utils::Logger::error("[/api/v1/chat/completions] 'purpose' parameter is missing.");
                nlohmann::json errJson{
                    {"error_code", 400},
                    {"details", "'purpose' parameter is required"}};
                callback(makeJSONResponse(errJson, 400));
                return;
            }

            // We'll store the string version to avoid repeated lookups
            const std::string purpose = bodyJson["purpose"].get<std::string>();

            // Choose logic based on "purpose"
            if (purpose == "discord-bot")
            {
                // --------------------------------------------------------------
                // Call Discord Bot handler
                // --------------------------------------------------------------
                auto resultJson = server::handleDiscordBotLLMQuery(bodyJson, fileParts);
                int httpCode = resultJson.value("ecode", 500);

                // Update metrics
                ++g_totalRequests;

                utils::Logger::info("[/api/v1/chat/completions] Responding with HTTP "
                                    + std::to_string(httpCode));
                callback(makeJSONResponse(resultJson, httpCode));
                return;
            }
            else if (purpose == "webpage")
            {
                // --------------------------------------------------------------
                // Placeholder for webpage logic
                // --------------------------------------------------------------
                nlohmann::json resultJson;
                resultJson["message"] = "Webpage placeholder not implemented yet";

                // Update metrics
                ++g_totalRequests;

                utils::Logger::info("[/api/v1/chat/completions] Responding with HTTP 200 (webpage placeholder).");
                callback(makeJSONResponse(resultJson, 200));
                return;
            }
            else
            {
                // --------------------------------------------------------------
                // No valid "purpose" found
                // --------------------------------------------------------------
                utils::Logger::error("[/api/v1/chat/completions] 'purpose' must be either 'discord-bot' or 'webpage'.");
                nlohmann::json errJson{
                    {"error_code", 400},
                    {"details", "'purpose' must be 'discord-bot' or 'webpage'"}};

                callback(makeJSONResponse(errJson, 400));
                return;
            }
        },
        {drogon::Post}
    );

    // ----------------------------------------------------------------------
    // GET /api/v1/status
    // ----------------------------------------------------------------------
    drogon::app().registerHandler(
        "/api/v1/status",
        [](const drogon::HttpRequestPtr&,
           std::function<void(const drogon::HttpResponsePtr&)>&& callback)
        {
            utils::Logger::info("[/api/v1/status] Received request.");

            nlohmann::json status;
            status["status_status"] = "normal";

            auto now = std::chrono::steady_clock::now();
            auto uptimeSec = std::chrono::duration_cast<std::chrono::seconds>(
                now - g_serverStartTime).count();
            status["uptime_seconds"] = uptimeSec;

#ifdef __linux__
            long rssKB = getMemoryUsageKB();
            if (rssKB >= 0)
            {
                long bytes = rssKB * 1024;
                status["memory_usage"] = formatMemorySizeBytes(bytes);
            }
            else
            {
                status["memory_usage"] = "unknown";
            }
#else
            status["memory_usage"] = "not available";
#endif
            status["build_version"] = "v1.0.0";

            // Generate a UTC timestamp
            auto t = std::time(nullptr);
            std::tm utcTm{};
#ifdef _WIN32
            gmtime_s(&utcTm, &t);
#else
            gmtime_r(&t, &utcTm);
#endif
            char timeBuf[64];
            std::strftime(timeBuf, sizeof(timeBuf), "%Y-%m-%dT%H:%M:%SZ", &utcTm);
            status["timestamp_utc"] = timeBuf;

            utils::Logger::info("[/api/v1/status] Responding with HTTP 200");
            callback(makeJSONResponse(status, 200));
        },
        {drogon::Get});

    // ----------------------------------------------------------------------
    // GET /metrics
    // ----------------------------------------------------------------------
    drogon::app().registerHandler(
        "/metrics",
        [](const drogon::HttpRequestPtr&,
           std::function<void(const drogon::HttpResponsePtr&)>&& callback)
        {
            utils::Logger::info("[/metrics] Received request.");

            std::ostringstream oss;
            oss << "# HELP llm_server_requests_total The total number of LLM requests processed.\n"
                << "# TYPE llm_server_requests_total counter\n"
                << "llm_server_requests_total "
                << g_totalRequests.load(std::memory_order_relaxed) << "\n";

            auto resp = drogon::HttpResponse::newHttpResponse();
            resp->setStatusCode(drogon::k200OK);
            resp->setContentTypeCode(drogon::CT_TEXT_PLAIN);
            resp->setBody(oss.str());

            utils::Logger::info("[/metrics] Responding with HTTP 200");
            callback(resp);
        },
        {drogon::Get});

    // ----------------------------------------------------------------------
    // GET /api/v1/logs
    // ----------------------------------------------------------------------
    drogon::app().registerHandler(
        "/api/v1/logs",
        [](const drogon::HttpRequestPtr& req,
           std::function<void(const drogon::HttpResponsePtr&)>&& callback)
        {
            utils::Logger::info("[/api/v1/logs] Received request.");

            int amount = 50; // default log amount
            auto amountStr = req->getParameter("amount");
            if (!amountStr.empty())
            {
                try
                {
                    amount = std::stoi(amountStr);
                }
                catch (...)
                {
                    utils::Logger::warn("[/api/v1/logs] Invalid amount param. Defaulting to 50.");
                }
            }

            auto recentLogs = utils::Logger::getRecentLogs(amount);

            // Build a JSON array of the log lines
            nlohmann::json logsJson = nlohmann::json::array();
            for (auto& logLine : recentLogs)
            {
                logsJson.push_back(logLine);
            }

            utils::Logger::info("[/api/v1/logs] Responding with last "
                                + std::to_string(amount) + " logs.");
            callback(makeJSONResponse(logsJson, 200));
        },
        {drogon::Get});

    // Configure server and start
    utils::Logger::info("Starting Drogon server on port 8080...");
    drogon::app().addListener("0.0.0.0", 8080);
    drogon::app().setThreadNum(std::thread::hardware_concurrency());
    drogon::app().run();
}

} // namespace server
