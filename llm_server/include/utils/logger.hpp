#ifndef LOGGER_HPP
#define LOGGER_HPP

#include <string>
#include <vector>

#define SERVER_LOG_PATH "/var/log/llm_server/server.log"

namespace utils {

class Logger {
public:
    // Log an info-level message
    static void info(const std::string& message);

    // Log an error-level message
    static void error(const std::string& message);

    // Log a warning-level message
    static void warn(const std::string& message);

    /**
     * Optional: Let you set the log file at runtime (if not using a hard-coded file).
     * Must be called before logging starts (or close & reopen).
     */
    static void setLogFile(const std::string& filename);

    /**
     * Return up to maxCount recent log messages from the in-memory buffer (newest last).
     * If the buffer has fewer logs than maxCount, returns all of them.
     */
    static std::vector<std::string> getRecentLogs(int maxCount);

private:
    Logger() = default;
    ~Logger() = default;
};

} // namespace utils

#endif // LOGGER_HPP
