#include "server/request_handler.hpp"
#include "models/openai_o3_mini.hpp"

// Other includes for other models
// #include "models/qingyunke.hpp"
// #include "models/anthropic.hpp"
// #include "models/gemini.hpp"
// #include "models/openai.hpp"
// #include "models/xai.hpp"

#include "utils/logger.hpp"
#include "utils/token_tracker.hpp"
#include <nlohmann/json.hpp>

namespace server {

nlohmann::json handleLLMQuery(const nlohmann::json& input) {
    nlohmann::json response = {
        {"model_used",  ""},
        {"message",     ""},
        {"files",       nlohmann::json::array()},
        {"token_usage", 0},
        {"error_code",  200},
        {"details",     ""}
    };

    // Example: we read a "model_name" or "model" from the input
    std::string modelName = input.value("model_name", "");
    // Could also be "openai_o3mini" or something you define
    if (modelName.empty()) {
        response["error_code"] = 400;
        response["details"]    = "No model_name provided.";
        return response;
    }

    // Convert "messages" array from input into a vector<ChatMessage>
    // Expecting something like:
    // {
    //   "model_name": "openai_o3mini",
    //   "messages": [
    //     {"role": "system",    "content": "You are a helpful assistant."},
    //     {"role": "user",      "content": "Hello, world."},
    //     {"role": "assistant", "content": "Hi, how can I help?"}
    //   ]
    // }
    std::vector<ChatMessage> messagesVec;
    if (input.contains("messages") && input["messages"].is_array()) {
        for (auto& m : input["messages"]) {
            ChatMessage cm;
            cm.role    = m.value("role", "");
            cm.content = m.value("content", "");
            messagesVec.push_back(cm);
        }
    }

    try {
        nlohmann::json modelResponse;

        if (modelName == "openai_o3_mini") {
            // Example usage: default reasoning_effort = "high"
            modelResponse = models::OpenAIo3mini::query(messagesVec, "high");
        } 
        else if (modelName == "openai") {
            // If you have a separate openai.cpp for “gpt-4” or other
            // modelResponse = models::OpenAI::query(...);
        }
        // else if (modelName == "anthropic") { ... }
        // else if (modelName == "qingyunke") { ... }
        // else if (modelName == "gemini") { ... }
        // else if (modelName == "xai") { ... }
        else {
            response["error_code"] = 400;
            response["details"]    = "Unrecognized model_name: " + modelName;
            return response;
        }

        // Merge modelResponse into your standardized response
        response["model_used"]  = modelResponse.value("model_used", modelName);
        response["message"]     = modelResponse.value("message", "");
        response["files"]       = modelResponse.value("files", nlohmann::json::array());
        response["token_usage"] = modelResponse.value("token_usage", 0);
        response["error_code"]  = modelResponse.value("error_code", 200);
        response["details"]     = modelResponse.value("details", "");

        // Optionally track tokens
        utils::TokenTracker::addUsage(response["token_usage"].get<int>());

    } catch (const std::exception& e) {
        utils::Logger::error("Error in handleLLMQuery: " + std::string(e.what()));
        response["error_code"]  = 500;
        response["details"]     = e.what();
        response["token_usage"] = 0;
    }

    return response;
}

} // namespace server
