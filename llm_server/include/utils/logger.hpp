#ifndef LOGGER_HPP
#define LOGGER_HPP

#include <string>
#include <vector>

#define SERVER_LOG_PATH "/var/log/llm_server/server.log"

namespace utils
{
/**
 * @brief A utility class for logging messages at various severity levels.
 *
 * This logger can optionally store recent messages in an in-memory buffer
 * and also write logs to a specified file if configured.
 */
class Logger
{
public:
    /**
     * @brief Logs a message at the INFO level.
     * @param message The message to log.
     */
    static void info(const std::string &message);

    /**
     * @brief Logs a message at the ERROR level.
     * @param message The message to log.
     */
    static void error(const std::string &message);

    /**
     * @brief Logs a message at the WARN level.
     * @param message The message to log.
     */
    static void warn(const std::string &message);

    /**
     * @brief Sets the log file at runtime.
     *
     * Call this before logging starts or after the logger file has been closed.
     * @param filename The absolute or relative path to the new log file.
     */
    static void setLogFile(const std::string &filename);

    /**
     * @brief Retrieves a specified number of recent log messages from the in-memory buffer.
     *
     * @param maxCount The maximum number of log messages to return.
     * @return A vector of strings containing the requested log messages (newest last).
     */
    static std::vector<std::string> getRecentLogs(int maxCount);

private:
    Logger() = default;
    ~Logger() = default;
};

} // namespace utils

#endif // LOGGER_HPP
