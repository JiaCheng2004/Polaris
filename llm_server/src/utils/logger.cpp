#include "utils/logger.hpp"
#include <iostream>
#include <chrono>
#include <ctime>

namespace utils {

void Logger::info(const std::string& message) {
    auto now = std::chrono::system_clock::to_time_t(std::chrono::system_clock::now());
    std::string timeStr = std::ctime(&now);
    timeStr.pop_back(); // remove newline
    std::cout << "[" << timeStr << "][INFO] " << message << std::endl;
}

void Logger::error(const std::string& message) {
    auto now = std::chrono::system_clock::to_time_t(std::chrono::system_clock::now());
    std::string timeStr = std::ctime(&now);
    timeStr.pop_back(); // remove newline
    std::cerr << "[" << timeStr << "][ERROR] " << message << std::endl;
}

} // namespace utils
