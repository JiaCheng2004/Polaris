#ifndef REQUEST_HANDLER_HPP
#define REQUEST_HANDLER_HPP

#include <nlohmann/json.hpp>
#include <string>

namespace server {

/**
 * A simple function to handle requests intended for the Qingyunke model.
 * This might expand into a larger dispatch system for multiple models.
 */
nlohmann::json handleQingyunke(const std::string& message);

} // namespace server

#endif // REQUEST_HANDLER_HPP
