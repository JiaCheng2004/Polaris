cmake_minimum_required(VERSION 3.14)
project(llm_server)

set(CMAKE_CXX_STANDARD 17)

# 1) Find the required Boost and nlohmann_json
find_package(Boost REQUIRED COMPONENTS system thread)
find_package(nlohmann_json REQUIRED)
find_package(CURL REQUIRED)

# 2) Fetch Crow from GitHub at configure time
include(FetchContent)

FetchContent_Declare(
  crow
  GIT_REPOSITORY https://github.com/CrowCpp/Crow.git
  GIT_TAG        v1.0+4    # Use a stable version or tag (adjust as needed)
)

# This will download Crow into crow_SOURCE_DIR and build it
FetchContent_MakeAvailable(crow)

# 3) Gather or list your source files
file(GLOB SERVER_SOURCES "src/server/*.cpp")
file(GLOB MODEL_SOURCES  "src/models/*.cpp")
file(GLOB UTILS_SOURCES  "src/utils/*.cpp")

# Alternatively, you can explicitly list them:
# set(SERVER_SOURCES
#     src/server/http_server.cpp
#     src/server/request_handler.cpp
# )
# set(MODEL_SOURCES
#     src/models/qingyunke.cpp
# )
# set(UTILS_SOURCES
#     src/utils/logger.cpp
#     src/utils/token_tracker.cpp
# )

# 4) Define your executable
add_executable(llm_server
    src/main.cpp
    ${SERVER_SOURCES}
    ${MODEL_SOURCES}
    ${UTILS_SOURCES}
)

# 5) Include directories
target_include_directories(llm_server PRIVATE
    ${Boost_INCLUDE_DIRS}
    ${CURL_INCLUDE_DIRS}
    ${CMAKE_SOURCE_DIR}/include
    ${crow_SOURCE_DIR}/include
)

# 6) Link libraries: Boost, nlohmann_json, and Crow
#   Depending on the Crow version, the library target might be 'crow', 'crow_all', etc.
#   Usually you'll see 'crow' or 'crow::crow'.
target_link_libraries(llm_server
    ${Boost_LIBRARIES}
    nlohmann_json::nlohmann_json
    CURL::libcurl
)