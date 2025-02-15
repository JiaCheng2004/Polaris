// include/models/gemini2_pro.hpp
#ifndef GOOGLE_GEMINI2_PRO_HPP
#define GOOGLE_GEMINI2_PRO_HPP

#include <string>
#include <vector>
#include <fstream>
#include <sstream>
#include <stdexcept>
#include <curl/curl.h>
#include <crow/multipart.h>
#include <nlohmann/json.hpp>
#include "utils/model_common.hpp"

using namespace models;

namespace models {

class GoogleGemini2Pro {
public:
    static ModelResult uploadAndQuery(const std::vector<ChatMessage>& messages,
                                      const std::vector<crow::multipart::part>& fileParts);
};

} // namespace models

#endif // GOOGLE_GEMINI2_PRO_HPP
