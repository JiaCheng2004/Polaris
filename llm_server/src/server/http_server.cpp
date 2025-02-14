#include "server/http_server.hpp"
#include "server/request_handler.hpp"
#include "utils/logger.hpp"

// For Crow (if you're using it):
#include <crow.h>
#include <nlohmann/json.hpp>

namespace server {

void startServer() {
    crow::SimpleApp app;

    CROW_ROUTE(app, "/api/v1/qingyunke")
    ([&](const crow::request& req){
        // Directly read the `msg` parameter
        std::string msg = req.url_params.get("msg") ? req.url_params.get("msg") : "";

        // Dispatch to request handler
        auto responseJson = server::handleQingyunke(msg);

        // Create a Crow response
        crow::response crowRes;
        crowRes.code = static_cast<int>(responseJson.value("error_code", 500));
        crowRes.set_header("Content-Type", "application/json");
        crowRes.body = responseJson.dump();

        return crowRes;
    });
    
    // Start the server on port 8080
    utils::Logger::info("Starting HTTP server on port 8080");
    app.port(8080).multithreaded().run();
}

} // namespace server
