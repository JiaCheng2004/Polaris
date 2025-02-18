// src/server/request_handler.cpp

#include "server/request_handler.hpp"
#include "utils/logger.hpp"
#include "utils/token_tracker.hpp"
#include "utils/model_result.hpp"

// Model headers
#include "models/openai_gpt_4.hpp"
#include "models/openai_gpt_4o.hpp"
#include "models/openai_o1.hpp"
#include "models/openai_o3_mini.hpp"
#include "models/google_gemini2_pro.hpp"

#include <nlohmann/json.hpp>
#include <crow.h>
#include <crow/multipart.h>

using namespace models;

namespace server {

/**
 * Handler that checks the model
 * and delegates everything else to the sub-model's functions.
 */
nlohmann::json handleLLMQuery(const nlohmann::json& input,
                              const std::vector<crow::multipart::part>& fileParts)
{
    // Log that we're entering the function
    utils::Logger::info("handleLLMQuery called with potential fileParts = " 
        + std::to_string(fileParts.size()));

    // Prepare a standard response JSON
    nlohmann::json response = {
        {"model",       ""},                      // (was model_used)
        {"message",     ""},
        {"files",       nlohmann::json::array()},
        {"token_used",  0},                       // (was token_usage)
        {"ecode",       200},                     // (was error_code)
        {"emessage",    ""},                      // (was details)
        {"model_info",  nlohmann::json::object()},
        {"additional",  nlohmann::json::object()}
    };

    // 1. Check for model
    std::string modelName = input.value("model", "");
    if (modelName.empty()) {
        utils::Logger::warn("No 'model' field provided in JSON request.");
        response["ecode"]    = 400;
        response["emessage"] = "No model was provided.";
        return response;
    }

    utils::Logger::info("Processing request for model: " + modelName);

    // 2. Route to the correct model
    ModelResult modelResult;
    try {
        if (modelName == "openai-gpt-4") {
            utils::Logger::info("Routing to OpenAIGPT4");
            modelResult = OpenAIGPT4().uploadAndQuery(input, fileParts);
        } 
        else if (modelName == "openai-gpt-4o") {
            utils::Logger::info("Routing to OpenAIGPT4o");
            modelResult = OpenAIGPT4o().uploadAndQuery(input, fileParts);
        } 
        else if (modelName == "openai-o1"
              || modelName == "openai-o1-low"
              || modelName == "openai-o1-medium"
              || modelName == "openai-o1-high")
        {
            utils::Logger::info("Routing to OpenAI-o1 variants");
            modelResult = OpenAIo1().uploadAndQuery(input, fileParts);
        }
        else if (modelName == "openai-o3-mini"
              || modelName == "openai-o3-mini-low"
              || modelName == "openai-o3-mini-medium"
              || modelName == "openai-o3-mini-high")
        {
            utils::Logger::info("Routing to OpenAI-o3-mini variants");
            modelResult = OpenAIo3mini().uploadAndQuery(input, fileParts);
        }
        else if (modelName == "google-gemini-2.0-pro") {
            utils::Logger::info("Routing to GoogleGemini2Pro");
            modelResult = GoogleGemini2Pro().uploadAndQuery(input, fileParts);
        } 
        else {
            // Unknown model
            utils::Logger::warn("Unrecognized model requested: " + modelName);
            modelResult.success      = false;
            modelResult.errorCode    = 400;
            modelResult.errorMessage = "Unrecognized model: " + modelName;
        }
    } catch (const std::exception& e) {
        utils::Logger::error("Exception caught in handleLLMQuery: " + std::string(e.what()));
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
        utils::Logger::info("Model succeeded; tracking token usage: "
                            + std::to_string(modelResult.tokenUsage));
        utils::TokenTracker::addUsage(modelResult.tokenUsage);
    } else {
        utils::Logger::warn("Model call failed with ecode="
                            + std::to_string(modelResult.errorCode)
                            + " : " + modelResult.errorMessage);
    }

    // Optionally log the final response summary
    utils::Logger::info("handleLLMQuery returning ecode=" 
                        + std::to_string(response["ecode"].get<int>())
                        + " for model=" + modelName);

    return response;
}

// Overload that doesn't accept fileParts
nlohmann::json handleLLMQuery(const nlohmann::json& input) {
    utils::Logger::info("handleLLMQuery called (no files).");
    std::vector<crow::multipart::part> empty;
    return handleLLMQuery(input, empty);
}

} // namespace server
