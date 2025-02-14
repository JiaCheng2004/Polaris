#ifndef QINGYUNKE_HPP
#define QINGYUNKE_HPP

#include <string>
#include <nlohmann/json.hpp>

namespace models {

/**
 * Qingyunke class integrates with http://api.qingyunke.com/api.php
 * Example usage:
 *    auto response = Qingyunke::query("你好");
 */
class Qingyunke {
public:
    /**
     * Sends a GET request with key=free, appid=0, and the given msg=<userMessage>.
     * Returns a JSON object:
     * {
     *   "model_used": "qingyunke",
     *   "message": "你好，我就开心了",
     *   "token_usage": 0, // or some value you compute
     *   "error_code": 200,
     *   "details": "..."
     * }
     */
    static nlohmann::json query(const std::string& userMessage);
};

} // namespace models

#endif // QINGYUNKE_HPP
