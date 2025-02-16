#include "models/google_gemini2_pro.hpp"

namespace models {

ModelResult GoogleGemini2Pro::uploadAndQuery(const std::string& context,
                                             const std::vector<crow::multipart::part>& fileParts)
{
    ModelResult result;
    result.success   = true;
    result.modelUsed = "google-gemini-2.0-pro";
    // Dummy message
    result.message   = "Hello from Google Gemini 2.0 Pro with no files allowed.";
    result.tokenUsage = 50;
    result.errorCode = 200; // OK
    
    return result;
}

} // namespace models
