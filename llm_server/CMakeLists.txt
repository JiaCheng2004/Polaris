# -------------------------------------------------------------------------
# Sets the minimum required version of CMake to 3.14.
# 设置 CMake 的最低要求版本为 3.14。
cmake_minimum_required(VERSION 3.14)

# -------------------------------------------------------------------------
# Defines the project name "llm_server" and sets the project scope.
# 定义名为 "llm_server" 的项目，并设置项目范围。
project(llm_server)

# -------------------------------------------------------------------------
# Enforces C++17 as the language standard for this project.
# 使用 C++17 作为项目的语言标准。
set(CMAKE_CXX_STANDARD 17)

# -------------------------------------------------------------------------
# 1) Find the required third-party libraries:
#    - Boost (with system and thread components)
#    - nlohmann_json
#    - CURL
# 1) 查找所需的第三方库：
#    - Boost（包含 system 和 thread 模块）
#    - nlohmann_json
#    - CURL
find_package(Boost REQUIRED COMPONENTS system thread)
find_package(nlohmann_json REQUIRED)
find_package(CURL REQUIRED)

# -------------------------------------------------------------------------
# 2) Find the Drogon framework.
# 2) 查找 Drogon 框架。
find_package(Drogon REQUIRED)

# -------------------------------------------------------------------------
# 3) Gather source files from specific directories:
#    - server/ directory
#    - models/ directory
#    - utils/ directory
# 3) 从特定目录中收集源文件：
#    - server/ 目录
#    - models/ 目录
#    - utils/ 目录
file(GLOB SERVER_SOURCES "src/server/*.cpp")
file(GLOB_RECURSE MODEL_SOURCES "src/models/*.cpp")
file(GLOB UTILS_SOURCES  "src/utils/*.cpp")

# -------------------------------------------------------------------------
# 4) Define the executable named "llm_server" and compile the gathered source files.
# 4) 定义名为 "llm_server" 的可执行程序，并编译收集到的源文件。
add_executable(llm_server
    src/main.cpp
    ${SERVER_SOURCES}
    ${MODEL_SOURCES}
    ${UTILS_SOURCES}
)

# -------------------------------------------------------------------------
# 5) Specify include directories for the target:
#    - Boost
#    - CURL
#    - and your project's /include
# 5) 指定目标的头文件目录：
#    - Boost
#    - CURL
#    - 以及项目的 /include 目录
target_include_directories(llm_server PRIVATE
    ${Boost_INCLUDE_DIRS}
    ${CURL_INCLUDE_DIRS}
    ${CMAKE_SOURCE_DIR}/include
)

# -------------------------------------------------------------------------
# 6) Link the required libraries to the "llm_server" executable:
#    - Boost libraries
#    - nlohmann_json
#    - CURL
#    - Drogon
# 6) 将所需的库链接到 "llm_server" 可执行文件：
#    - Boost 库
#    - nlohmann_json
#    - CURL
#    - Drogon
target_link_libraries(llm_server
    ${Boost_LIBRARIES}
    nlohmann_json::nlohmann_json
    CURL::libcurl
    Drogon::Drogon
)
