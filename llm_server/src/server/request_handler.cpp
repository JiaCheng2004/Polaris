// src/server/request_handler.cpp

#include "server/request_handler.hpp"
#include "utils/logger.hpp"
#include "utils/token_tracker.hpp"
#include "utils/model_result.hpp"

#include "models/openai_gpt_4.hpp"
#include "models/openai_gpt_4o.hpp"
#include "models/openai_o1.hpp"
#include "models/openai_o3.hpp"
#include "models/google_gemini2_pro.hpp"

#include <nlohmann/json.hpp>
#include <crow.h>
#include <crow/multipart.h>

using namespace models;

namespace server {

/**
 * Handler that checks the model, retrieves the context if desired,
 * and delegates everything else to the sub-model's functions.
 */
nlohmann::json handleLLMQuery(const nlohmann::json& input,
                              const std::vector<crow::multipart::part>& fileParts)
{
    // Prepare a standard response JSON
    // Renamed keys + added model_info & additional objects
    nlohmann::json response = {
        {"model",       ""},                              // (was model_used)
        {"message",     ""},
        {"files",       nlohmann::json::array()},
        {"token_used",  0},                               // (was token_usage)
        {"ecode",       200},                             // (was error_code)
        {"emessage",    ""},                              // (was details)
        {"model_info",  nlohmann::json::object()},        // newly added
        {"additional",  nlohmann::json::object()}         // newly added
    };

    // 1. Check for model
    std::string modelName = input.value("model", "");
    if (modelName.empty()) {
        response["ecode"]    = 400;
        response["emessage"] = "No model provided.";
        return response;
    }

    // If you want to pass a "context" field to sub-models, grab it here
    std::string context = input.value("context", "");

    // 2. Route to the correct model
    ModelResult modelResult;
    try {
        if (modelName == "openai-gpt-4") {
            modelResult = OpenAIGPT4::uploadAndQuery(context, fileParts);
        } else if (modelName == "openai-gpt-4o") {
            modelResult = OpenAIGPT4o::uploadAndQuery(context, fileParts);
        } else if (modelName == "openai-o1-low") {
            modelResult = OpenAIo1::uploadAndQuery(context, fileParts);
        } else if (modelName == "openai-o1-medium") {
            modelResult = OpenAIo1::uploadAndQuery(context, fileParts);
        } else if (modelName == "openai-o1-high") {
            modelResult = OpenAIo1::uploadAndQuery(context, fileParts);
        } else if (modelName == "openai-o3-mini-low") {
            modelResult = OpenAIo3mini::uploadAndQuery(context, fileParts);
        } else if (modelName == "openai-o3-mini-medium") {
            modelResult = OpenAIo3mini::uploadAndQuery(context, fileParts);
        } else if (modelName == "openai-o3-mini-high") {
            modelResult = OpenAIo3mini::uploadAndQuery(context, fileParts);
        } else if (modelName == "google-gemini-2.0-pro") {
            modelResult = GoogleGemini2Pro::uploadAndQuery(context, fileParts);
        } else {
            // Unknown model
            modelResult.success      = false;
            modelResult.errorCode    = 400;
            modelResult.errorMessage = "Unrecognized model: " + modelName;
        }
    } catch (const std::exception& e) {
        modelResult.success      = false;
        modelResult.errorCode    = 500;
        modelResult.errorMessage = std::string("Exception: ") + e.what();
    }

    // 3. Convert ModelResult -> standardized JSON response
    response["model"]      = modelResult.modelUsed;  
    response["message"]    = modelResult.message;
    response["token_used"] = modelResult.tokenUsage;
    response["ecode"]      = modelResult.errorCode;
    if (!modelResult.success) {
        response["emessage"] = modelResult.errorMessage;
    }

    // If any file IDs were returned, store them in response
    for (auto& fID : modelResult.fileIds) {
        nlohmann::json f;
        f["file_id"] = fID;
        response["files"].push_back(f);
    }

    // 4. If model succeeded, track tokens
    if (modelResult.success) {
        utils::TokenTracker::addUsage(modelResult.tokenUsage);
    }

    // In the future, you could populate "model_info" or "additional" fields:
    // response["model_info"] = { {"param1", "value"}, {"param2", 42} };
    // response["additional"] = { {"hint", "some future usage"}, {"debug", "..."} };

    return response;
}

// Overload that doesn't accept fileParts
nlohmann::json handleLLMQuery(const nlohmann::json& input) {
    std::vector<crow::multipart::part> empty;
    return handleLLMQuery(input, empty);
}

} // namespace server
