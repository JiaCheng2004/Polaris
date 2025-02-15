#ifndef OPENAI_O3_MINI_HPP
#define OPENAI_O3_MINI_HPP

#include <string>
#include <vector>
#include <crow/multipart.h>
#include <nlohmann/json.hpp>
#include "utils/model_common.hpp"  // Contains models::ModelResult, models::ChatMessage, etc.

namespace models
{

/**
 * A structure to encapsulate the result of a single file-upload attempt
 * to the OpenAI Files endpoint.
 * 
 * We keep it here in the SAME namespace as OpenAIo3mini,
 * so references like `models::UploadResult` are consistent.
 */
struct UploadResult {
    bool        success;      // True if file was uploaded successfully
    std::string fileId;       // Returned by OpenAI (e.g. "file-abc123")
    std::string errorReason;  // Explanation if unsuccessful
};

/**
 * OpenAIo3mini class for calling your GPT-based model with optional file uploads.
 */
class OpenAIo3mini {
public:
    /**
     * Uploads any provided fileParts to OpenAI, then calls ChatCompletion.
     *
     * @param messages        Vector of ChatMessage objects (roles: system/user/assistant).
     * @param fileParts       Zero or more uploaded files from user (multipart).
     * @param reasoningEffort e.g. "high", "medium", "low"
     *
     * @return ModelResult with success/failure, message, token usage, etc.
     */
    static ModelResult uploadAndQuery(
        const std::vector<ChatMessage>& messages,
        const std::vector<crow::multipart::part>& fileParts,
        const std::string& reasoningEffort
    );

private:
    /**
     * Uploads a single file to OpenAI's File endpoint with multipart/form-data.
     *
     * @param filePath Path on disk for the file to upload.
     * @param filename The name shown in the server side (Content-Disposition).
     * @param apiKey   Your OpenAI API key.
     * @param purpose  The "purpose" field (default "assistants").
     *
     * @return UploadResult with success/failure and file ID.
     */
    static UploadResult uploadFileOpenAI(
        const std::string& filePath,
        const std::string& filename,
        const std::string& apiKey,
        const std::string& purpose = "assistants"
    );
};

} // end namespace models

#endif // OPENAI_O3_MINI_HPP
