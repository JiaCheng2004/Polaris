#include "server/http_server.hpp"
#include "server/request_handler.hpp"
#include "utils/logger.hpp"
#include "utils/multipart_utils.hpp"
#include <nlohmann/json.hpp>
#include <string>
#include <vector>

#include <crow.h>
#include <crow/multipart.h>

namespace server {

void startServer() {
    crow::SimpleApp app;

    CROW_ROUTE(app, "/api/v1/llm/completions")
        .methods(crow::HTTPMethod::POST)
    ([&](const crow::request& req){
        crow::response res;
        res.set_header("Content-Type", "application/json");

        // We'll keep track of any files in this vector
        std::vector<crow::multipart::part> fileParts;

        // We'll store JSON data here
        nlohmann::json bodyJson;
        bool jsonParsed = false;

        auto contentType = req.get_header_value("Content-Type");
        CROW_LOG_INFO << "Content-Type from client: " << contentType;

        // Attempt to parse the request as multipart
        crow::multipart::message multipartReq(req);
        CROW_LOG_INFO << "multipartReq.parts.size() = " << multipartReq.parts.size();

        // If we have multipart parts, iterate over them
        if (!multipartReq.parts.empty()) {
            // We have at least some multipart fields
            for (auto& field : multipartReq.parts) {
                for (auto& kv : field.headers) {
                    CROW_LOG_INFO << "Header: " << kv.first << " => " << kv.second.value;
                }
                // Retrieve the "name" from Content-Disposition
                std::string fieldName = utils::getPartName(field);
                CROW_LOG_INFO << "Computed fieldName = '" << fieldName << "'";

                if (fieldName == "files") {
                    // This is a file upload field
                    fileParts.push_back(field);
                } else if (fieldName == "json") {
                    // This might be the JSON data (the user put JSON in a part named "json")
                    try {
                        bodyJson = nlohmann::json::parse(field.body);
                        jsonParsed = true;
                    } catch(...) {
                        res.code = 400;
                        res.body = R"({"error_code":400,"details":"Invalid JSON in 'json' part"})";
                        return res;
                    }
                }
            }
        }

        // If we still have not parsed JSON, maybe the user sent raw JSON (not multipart)
        if (!jsonParsed) {
            try {
                bodyJson = nlohmann::json::parse(req.body);
            } catch(...) {
                // Invalid JSON
                res.code = 400;
                res.body = R"({"error_code":400,"details":"Invalid JSON in request body"})";
                return res;
            }
        }

        // Now call the handler with the JSON and the file parts
        CROW_LOG_INFO << "About to call handleLLMQuery...";
        auto resultJson = server::handleLLMQuery(bodyJson, fileParts);
        CROW_LOG_INFO << "Successfully returned from handleLLMQuery...";
        CROW_LOG_INFO << resultJson;
        res.code = resultJson.value("error_code", 500);
        res.body = resultJson.dump();
        return res;
    });

    // Optional health-check route
    CROW_ROUTE(app, "/health")
    ([](){
        nlohmann::json health;
        health["status"] = "ok";
        return crow::response{health.dump()};
    });

    utils::Logger::info("Starting server on port 8080...");
    app.port(8080).multithreaded().run();
}

} // namespace server
