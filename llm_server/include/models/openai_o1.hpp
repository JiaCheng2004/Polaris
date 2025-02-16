#ifndef OPENAI_O1_HPP
#define OPENAI_O1_HPP

#include "utils/model_result.hpp"
#include <crow/multipart.h>
#include <string>
#include <vector>

namespace models {

class OpenAIo1
{
public:
    static ModelResult uploadAndQuery(const std::string& context,
                                      const std::vector<crow::multipart::part>& fileParts);
};

} // namespace models

#endif // OPENAI_O1_HPP
