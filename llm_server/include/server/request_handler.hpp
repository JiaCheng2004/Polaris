#ifndef REQUEST_HANDLER_HPP
#define REQUEST_HANDLER_HPP

#include <nlohmann/json.hpp>
#include <string>

namespace server {
    nlohmann::json handleLLMQuery(const nlohmann::json& input);
}


#endif // REQUEST_HANDLER_HPP
