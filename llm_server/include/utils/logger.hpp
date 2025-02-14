#ifndef LOGGER_HPP
#define LOGGER_HPP

#include <string>

namespace utils {

class Logger {
public:
    // Log an info-level message
    static void info(const std::string& message);

    // Log an error-level message
    static void error(const std::string& message);

private:
    Logger() = default;
    ~Logger() = default;
};

} // namespace utils

#endif // LOGGER_HPP
