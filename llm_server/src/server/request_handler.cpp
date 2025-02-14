#include "server/request_handler.hpp"
#include "models/qingyunke.hpp"
#include "utils/logger.hpp"
#include "utils/token_tracker.hpp"

namespace server {

nlohmann::json handleQingyunke(const std::string& message) {
    // Initialize a standard JSON response
    nlohmann::json response;
    response["model_used"]  = "qingyunke";
    response["message"]     = "";
    response["files"]       = nlohmann::json::array();
    response["token_usage"] = 0;
    response["error_code"]  = 200;
    response["details"]     = "";

    try {
        // Query the Qingyunke model
        auto apiResponse = models::Qingyunke::query(message);

        // We can merge or override fields from the model’s response
        response["model_used"]   = apiResponse.value("model_used", "qingyunke");
        response["message"]      = apiResponse.value("message", "");
        response["token_usage"]  = apiResponse.value("token_usage", 0);
        response["error_code"]   = apiResponse.value("error_code", 500);
        response["details"]      = apiResponse.value("details", "");

        // Optionally track tokens
        utils::TokenTracker::addUsage(response["token_usage"].get<int>());

    } catch (const std::exception& e) {
        response["token_usage"] = 0;
        response["error_code"]  = 500;
        response["details"]     = e.what();
        utils::Logger::error("Error in handleQingyunke: " + std::string(e.what()));
    }

    return response;
}

} // namespace server
