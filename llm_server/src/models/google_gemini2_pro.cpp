// src/models/gemini2_pro.cpp

#include "models/google_gemini2_pro.hpp"

namespace models {

ModelResult GoogleGemini2Pro::uploadAndQuery(const std::vector<ChatMessage>& messages,
                                       const std::vector<crow::multipart::part>& fileParts)
{
    ModelResult result;
    result.success      = true;
    result.errorCode    = 200;
    result.errorMessage = "";
    result.modelUsed    = "google-gemini-2.0-pro";
    result.message      = "";
    result.tokenUsage   = 0;

    // For demonstration, let's say GoogleGemini2Pro doesn't support file uploads at all:
    if (!fileParts.empty()) {
        result.success = false;
        result.errorCode = 400;
        result.errorMessage = "Google Gemini 2.0 Pro does not support file uploads.";
        return result;
    }

    // If it doesn't support files, just call the "Google Gemini 2.0 Pro" API for text:
    // (Dummy)
    result.message = "Hello from Google Gemini 2.0 Pro with no files allowed.";
    result.tokenUsage = 50;

    return result;
}

} // namespace models
