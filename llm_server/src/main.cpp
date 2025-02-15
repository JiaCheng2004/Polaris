#include "server/http_server.hpp"
#include "utils/logger.hpp"

#include <nlohmann/json.hpp>
#include <fstream>
#include <iostream>
#include <stdexcept>

// Declare a global config object
nlohmann::json g_config;

int main(int argc, char* argv[]) {
    try {
        // 1) Load config.json once
        std::ifstream ifs("config/config.json");
        if (!ifs.is_open()) {
            throw std::runtime_error("Cannot open config/config.json");
        }
        ifs >> g_config; // Populate the global JSON
        ifs.close();

        // 2) Log that we're starting
        utils::Logger::info("Starting LLM Server...");

        // 3) Start the HTTP server (Crow run loop)
        server::startServer(); // This call typically blocks until the server exits

    } catch (const std::exception& e) {
        std::cerr << "Fatal error: " << e.what() << std::endl;
        return 1;
    }

    return 0;
}
