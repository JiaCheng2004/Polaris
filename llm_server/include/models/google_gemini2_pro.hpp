#ifndef GOOGLE_GEMINI2_PRO_HPP
#define GOOGLE_GEMINI2_PRO_HPP

#include "utils/model_result.hpp"
#include <crow/multipart.h>
#include <string>
#include <vector>

namespace models {

class GoogleGemini2Pro
{
public:
    static ModelResult uploadAndQuery(const std::string& context,
                                      const std::vector<crow::multipart::part>& fileParts);
};

} // namespace models

#endif // GOOGLE_GEMINI2_PRO_HPP
