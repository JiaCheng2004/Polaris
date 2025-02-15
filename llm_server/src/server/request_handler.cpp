// src/server/request_handler.cpp

#include "server/request_handler.hpp"
#include "models/openai_o3_mini.hpp"
#include "models/google_gemini2_pro.hpp" // or anthropic, claude, etc.
#include "utils/logger.hpp"
#include "utils/token_tracker.hpp"

#include <nlohmann/json.hpp>
#include <crow/multipart.h>

using namespace models;

namespace server {

/**
 * We'll overload handleLLMQuery to also receive fileParts.
 */
nlohmann::json handleLLMQuery(const nlohmann::json& input,
                              const std::vector<crow::multipart::part>& fileParts)
{
    // Prepare a standard response JSON
    nlohmann::json response = {
        {"model_used",  ""},
        {"message",     ""},
        {"files",       nlohmann::json::array()},  // We might fill with file info
        {"token_usage", 0},
        {"error_code",  200},
        {"details",     ""}
    };

    std::string modelName = input.value("model_name", "");
    if (modelName.empty()) {
        response["error_code"] = 400;
        response["details"]    = "No model_name provided.";
        return response;
    }

    // Convert JSON "messages" to vector<ChatMessage>
    std::vector<ChatMessage> messagesVec;
    if (input.contains("messages") && input["messages"].is_array()) {
        for (auto& m : input["messages"]) {
            ChatMessage cm;
            cm.role    = m.value("role", "");
            cm.content = m.value("content", "");
            messagesVec.push_back(cm);
        }
    }

    // We'll hold the final model result in this
    ModelResult modelResult;

    // 2. Route to the correct model
    try {
        if (modelName == "openai_o3_mini_high") {
            modelResult = OpenAIo3mini::uploadAndQuery(messagesVec, fileParts, "high");
        } else if (modelName == "openai_o3_mini_medium") {
            modelResult = OpenAIo3mini::uploadAndQuery(messagesVec, fileParts, "medium");
        } else if (modelName == "openai_o3_mini_low") {
            modelResult = OpenAIo3mini::uploadAndQuery(messagesVec, fileParts, "low");
        } else if (modelName == "google_gemini_2.0_pro") {
            modelResult = GoogleGemini2Pro::uploadAndQuery(messagesVec, fileParts);
        } 
        // else if (modelName == "anthropic_claude3") { ... }
        // else if (modelName == "xai_model")         { ... }
        else {
            // Unknown model
            modelResult.success      = false;
            modelResult.errorCode    = 400;
            modelResult.errorMessage = "Unrecognized model_name: " + modelName;
        }
    } catch (const std::exception& e) {
        modelResult.success      = false;
        modelResult.errorCode    = 500;
        modelResult.errorMessage = std::string("Exception: ") + e.what();
    }

    // 3. Convert ModelResult -> standardized JSON
    response["model_used"]  = modelResult.modelUsed;
    response["message"]     = modelResult.message;
    response["token_usage"] = modelResult.tokenUsage;
    response["error_code"]  = modelResult.errorCode;
    if (!modelResult.success) {
        response["details"] = modelResult.errorMessage;
    }

    // If any file IDs were returned, we can store them in response["files"].
    for (auto& fID : modelResult.fileIds) {
        nlohmann::json f;
        f["file_id"] = fID;
        response["files"].push_back(f);
    }

    // 4. If model succeeded, track tokens
    if (modelResult.success) {
        utils::TokenTracker::addUsage(modelResult.tokenUsage);
    }

    return response;
}

// Overload that uses an empty vector for fileParts if not given
nlohmann::json handleLLMQuery(const nlohmann::json& input) {
    std::vector<crow::multipart::part> empty;
    return handleLLMQuery(input, empty);
}

} // namespace server
