#include "server/http_server.hpp"
#include "server/request_handler.hpp"
#include "utils/logger.hpp"

#include <crow.h>
#include <nlohmann/json.hpp>
#include <string>

namespace server {

void startServer() {
    // Create a Crow application
    crow::SimpleApp app;

    // Example route: POST /api/v1/llm/query
    // You can define any other routes you need, e.g., GET /health
    CROW_ROUTE(app, "/api/v1/llm/query")
        .methods(crow::HTTPMethod::POST)
    ([&](const crow::request& req){
        // Parse the incoming JSON body
        auto bodyJson = nlohmann::json::parse(req.body, nullptr, false);
        if (bodyJson.is_discarded()) {
            // Invalid JSON
            crow::response res;
            res.code = 400;
            res.set_header("Content-Type", "application/json");
            res.body = R"({"error_code":400,"details":"Invalid JSON in request body"})";
            return res;
        }

        // Pass the request body to request handler
        auto result = server::handleLLMQuery(bodyJson);

        // Build the Crow response based on the handler result
        crow::response crowRes;
        crowRes.code = result.value("error_code", 500);
        crowRes.set_header("Content-Type", "application/json");
        crowRes.body = result.dump();

        return crowRes;
    });

    // (Optional) Example route: GET /health
    CROW_ROUTE(app, "/health")
    ([](){
        nlohmann::json health;
        health["status"] = "ok";
        return crow::response{health.dump()};
    });

    // Log server startup
    utils::Logger::info("Starting server on port 8080...");

    // Run the server (blocks indefinitely unless an exception or shutdown signal occurs)
    app.port(8080).multithreaded().run();
}

} // namespace server
