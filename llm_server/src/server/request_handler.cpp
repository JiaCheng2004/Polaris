#include "server/request_handler.hpp"
#include "utils/logger.hpp"
#include "utils/token_tracker.hpp"
#include "utils/model_result.hpp"
#include "utils/multipart_utils.hpp"
#include "models/openai_gpt_4.hpp"

#include <nlohmann/json.hpp>
#include <string>
#include <vector>

using namespace models;

namespace server
{

/**
 * @brief Handles incoming requests for language model queries.
 *
 * This function parses the input JSON and optional file attachments, 
 * then routes the request to the appropriate model. It ultimately
 * returns a standardized JSON response indicating success/failure
 * and any relevant output from the model.
 *
 * @param input     The JSON input that contains model parameters (e.g., "model", "prompt").
 * @param fileParts A list of file attachments (if any) uploaded with the request.
 * @return A JSON object containing the query result, including error codes or model output.
 */
nlohmann::json handleLLMQuery(
    const nlohmann::json &input,
    const std::vector<utils::MultipartPart> &fileParts)
{
    // Log basic call information
    utils::Logger::info("[handleLLMQuery] Invoked. Number of file parts: "
                        + std::to_string(fileParts.size()));

    // Prepare a standard response JSON structure
    nlohmann::json response = {
        {"model",       ""},
        {"message",     ""},
        {"files",       nlohmann::json::array()},
        {"token_used",  0},
        {"ecode",       200},
        {"emessage",    ""},
        {"model_info",  nlohmann::json::object()},
        {"additional",  nlohmann::json::object()}
    };

    // Extract and validate the model name
    std::string modelName = input.value("model", "");
    if (modelName.empty())
    {
        utils::Logger::warn("[handleLLMQuery] No 'model' field provided in input JSON.");
        response["ecode"]    = 400;
        response["emessage"] = "No model was provided.";
        return response;
    }

    utils::Logger::info("[handleLLMQuery] Request for model: " + modelName);

    // Route request to the appropriate model
    ModelResult modelResult;
    try
    {
        if (modelName == "openai-gpt-4")
        {
            utils::Logger::info("[handleLLMQuery] Routing to OpenAIGPT4.");
            modelResult = OpenAIGPT4().uploadAndQuery(input, fileParts);
        }
        else
        {
            // Handle unknown model
            utils::Logger::warn("[handleLLMQuery] Unrecognized model: " + modelName);
            modelResult.success      = false;
            modelResult.errorCode    = 400;
            modelResult.errorMessage = "Unrecognized model: " + modelName;
        }
    }
    catch (const std::exception &e)
    {
        // Log any exceptions
        utils::Logger::error("[handleLLMQuery] Exception caught: " + std::string(e.what()));
        modelResult.success      = false;
        modelResult.errorCode    = 500;
        modelResult.errorMessage = "Exception: " + std::string(e.what());
    }

    // Convert the ModelResult into our standardized JSON response
    response["model"]      = modelResult.modelUsed;
    response["message"]    = modelResult.message;
    response["token_used"] = modelResult.tokenUsage;
    response["ecode"]      = modelResult.errorCode;

    if (!modelResult.success)
    {
        response["emessage"] = modelResult.errorMessage;
    }

    // If any files were returned, add them to the response
    for (const auto &fID : modelResult.fileIds)
    {
        nlohmann::json f;
        f["file_id"] = fID;
        response["files"].push_back(f);
    }

    // Track token usage if the model was successful
    if (modelResult.success)
    {
        utils::Logger::info("[handleLLMQuery] Model succeeded; tracking token usage: "
                            + std::to_string(modelResult.tokenUsage));
        utils::TokenTracker::addUsage(modelResult.tokenUsage);
    }
    else
    {
        utils::Logger::warn("[handleLLMQuery] Model call failed. ecode="
                            + std::to_string(modelResult.errorCode)
                            + " | " + modelResult.errorMessage);
    }

    // Final log before returning
    utils::Logger::info("[handleLLMQuery] Returning ecode="
                        + std::to_string(response["ecode"].get<int>())
                        + " for model=" + modelName);

    return response;
}

} // namespace server
