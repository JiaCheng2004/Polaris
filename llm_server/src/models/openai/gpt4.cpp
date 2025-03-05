#include "models/openai/gpt4.hpp"
#include "utils/logger.hpp"

#include <fstream>
#include <filesystem>
#include <string>
#include <vector>

namespace fs = std::filesystem;

namespace models
{

/**
 * @brief Implements file handling and model invocation logic for OpenAI GPT-4.
 *
 * This method writes uploaded file parts to a temporary directory, then
 * proceeds with any necessary OpenAI-specific logic. It returns a
 * ModelResult object containing success or error data.
 *
 * @param input     The JSON input containing model request details.
 * @param fileParts A list of file attachments (if any) uploaded with the request.
 * @return A ModelResult describing success/failure and any model output.
 */
ModelResult OpenAIGPT4::uploadAndQuery(
    const nlohmann::json &input,
    const std::vector<utils::MultipartPart> &fileParts)
{
    utils::Logger::info("[OpenAIGPT4::uploadAndQuery] Called. Number of file parts: "
                        + std::to_string(fileParts.size()));

    ModelResult result;
    result.modelUsed = "openai-gpt-4";

    // Ensure a temporary directory exists
    fs::path tmpPath = "/tmp/llm_server";
    if (!fs::exists(tmpPath))
    {
        utils::Logger::warn("[OpenAIGPT4::uploadAndQuery] Directory does not exist; creating: "
                            + tmpPath.string());
        try
        {
            fs::create_directories(tmpPath);
        }
        catch (const fs::filesystem_error &ex)
        {
            utils::Logger::error("[OpenAIGPT4::uploadAndQuery] Failed to create directory: "
                                 + tmpPath.string() + ". Error: " + ex.what());
            result.success      = false;
            result.errorCode    = 500;
            result.errorMessage = "Failed to create necessary directories.";
            return result;
        }
    }

    // Write each uploaded file to disk
    for (const auto &part : fileParts)
    {
        fs::path filePath = tmpPath / part.filename;
        std::ofstream ofs(filePath, std::ios::binary);
        if (!ofs)
        {
            utils::Logger::error("[OpenAIGPT4::uploadAndQuery] Could not open file for writing: "
                                 + filePath.string());
            // File writing error can be critical or logged; optionally set success = false
            continue;
        }
        ofs.write(part.body.data(), static_cast<std::streamsize>(part.body.size()));
        ofs.close();

        utils::Logger::info("[OpenAIGPT4::uploadAndQuery] Saved file: "
                            + filePath.string()
                            + " | Content type: " + part.contentType
                            + " | File size: " + std::to_string(part.body.size()) + " bytes");
    }

    // ------------------------------------------------------
    // Placeholder for further model logic or API calls
    // ------------------------------------------------------

    // Return success if no major errors occurred
    result.success      = true;
    result.errorCode    = 200;
    result.errorMessage = "";

    utils::Logger::info("[OpenAIGPT4::uploadAndQuery] Completed processing files for openai-gpt-4.");
    return result;
}

} // namespace models
