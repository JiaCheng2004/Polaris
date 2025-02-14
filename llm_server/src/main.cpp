#include "server/http_server.hpp"
#include "utils/logger.hpp"

#include <iostream>

int main(int argc, char* argv[]) {
    try {
        // If you have a config file to load, you could do:
        // utils::ConfigManager::instance().load("config/config.json");

        utils::Logger::info("Starting LLM Server...");
        server::startServer(); // This will block (Crow run loop)

    } catch (const std::exception& e) {
        std::cerr << "Fatal error: " << e.what() << std::endl;
        return 1;
    }

    return 0;
}
