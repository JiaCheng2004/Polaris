#ifndef OPENAI_GPT_4_HPP
#define OPENAI_GPT_4_HPP

#include "utils/model_result.hpp"
#include <crow/multipart.h>
#include <string>
#include <vector>

namespace models {

class OpenAIGPT4
{
public:
    static ModelResult uploadAndQuery(const std::string& context,
                                      const std::vector<crow::multipart::part>& fileParts);
};

} // namespace models

#endif // OPENAI_GPT_4_HPP
