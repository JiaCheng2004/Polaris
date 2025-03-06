#include "server/discord_request.hpp"
#include "utils/logger.hpp"
#include "utils/token_tracker.hpp"
#include "utils/model_result.hpp"
#include "utils/multipart_utils.hpp"
#include "models/openai/gpt4o.hpp"

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
 * @param input     The JSON input that contains model parameters (e.g., "provider", "model", and "messages").
 * @param fileParts A list of file attachments (if any) uploaded with the request.
 * @return A JSON object containing the query result, including error codes or model output.
 */
nlohmann::json handleDiscordBotLLMQuery(
    const nlohmann::json &input,
    const std::vector<utils::MultipartPart> &fileParts)
{
    // Log basic call information
    utils::Logger::info("[handleDiscordBotLLMQuery] Invoked. Number of file parts: "
                        + std::to_string(fileParts.size()));

    // Prepare a standard response JSON structure
    nlohmann::json response = {
        {"model",       ""},
        {"result",     ""},
        {"files",       nlohmann::json::array()},
        {"token_used",  0},
        {"code",       200},
        {"message",    ""},
        {"model_info",  nlohmann::json::object()},
        {"additional",  nlohmann::json::object()}
    };

    // Extract and validate the model name
    std::string provider = input.value("provider", "");
    std::string modelName = input.value("model", "");
    if (modelName.empty())
    {
        utils::Logger::warn("[handleDiscordBotLLMQuery] No 'model' field provided in input JSON.");
        response["code"]    = 400;
        response["message"] = "No model was provided.";
        return response;
    }

    utils::Logger::info("[handleDiscordBotLLMQuery] Request for model: " + modelName);

    // Route request to the appropriate model
    ModelResult modelResult;
    try
    {
        if (modelName == "gpt4o")
        {
            utils::Logger::info("[handleDiscordBotLLMQuery] Routing to OpenAIGPT4o.");
            modelResult = OpenAIGPT4o().uploadAndQuery(input, fileParts);
        }
        else
        {
            // Handle unknown model
            utils::Logger::warn("[handleDiscordBotLLMQuery] Unrecognized model: " + modelName);
            modelResult.success = false;
            modelResult.code    = 400;
            modelResult.message = "Unrecognized model: " + modelName;
        }
    }
    catch (const std::exception &e)
    {
        // Log any exceptions
        utils::Logger::error("[handleDiscordBotLLMQuery] Exception caught: " + std::string(e.what()));
        modelResult.success = false;
        modelResult.code    = 500;
        modelResult.message = "Exception: " + std::string(e.what());
    }

    // Convert the ModelResult into our standardized JSON response
    response["model"]      = modelResult.model_used;
    response["result"]    = modelResult.result;
    response["token_used"] = modelResult.token_usage;
    response["code"]      = modelResult.code;

    if (!modelResult.success)
    {
        response["message"] = modelResult.message;
    }

    // If any files were returned, add them to the response
    for (const auto &fID : modelResult.file_ids)
    {
        nlohmann::json f;
        f["file_id"] = fID;
        response["files"].push_back(f);
    }

    // Track token usage if the model was successful
    if (modelResult.success)
    {
        utils::Logger::info("[handleDiscordBotLLMQuery] Model succeeded; tracking token usage: "
                            + std::to_string(modelResult.token_usage));
        utils::TokenTracker::addUsage(modelResult.token_usage);
    }
    else
    {
        utils::Logger::warn("[handleDiscordBotLLMQuery] Model call failed. code="
                            + std::to_string(modelResult.code)
                            + " | " + modelResult.message);
    }

    // Final log before returning
    utils::Logger::info("[handleDiscordBotLLMQuery] Returning code="
                        + std::to_string(response["code"].get<int>())
                        + " for model=" + modelName);

    return response;
}

} // namespace server
