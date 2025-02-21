# -------------------------------------------------------------------------
# 1) Sets the minimum required version of CMake to 3.14.
#    设置 CMake 的最低要求版本为 3.14。
cmake_minimum_required(VERSION 3.14)

# -------------------------------------------------------------------------
# 2) Defines the project name "marong".
#    定义名为 "marong" 的项目。
project(marong)

# -------------------------------------------------------------------------
# 3) Enforces C++20 as the language standard for this project
#    and enables position-independent code.
#    使用 C++20 作为项目的语言标准，并启用位置无关代码。
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

# -------------------------------------------------------------------------
# 4) Find the required libraries:
#    - Threads
#    - OpenSSL
#    - CURL
#    查找所需的库：
#    - 线程库 (Threads)
#    - OpenSSL
#    - CURL
find_package(Threads REQUIRED)
find_package(OpenSSL REQUIRED)
find_package(CURL REQUIRED)

# -------------------------------------------------------------------------
# 5) Fetch D++ (DPP) from GitHub via FetchContent.
#    Note: GIT_TAG "master" means the latest code from the main branch.
#    If you prefer a stable release, specify a tagged version (e.g., "v10.0.12").
#    使用 FetchContent 从 GitHub 获取 D++ (DPP)。
#    注意：GIT_TAG 为 "master" 会获取主分支最新代码，如需稳定版可指定发行标签。
include(FetchContent)
FetchContent_Declare(
    libdpp
    GIT_REPOSITORY https://github.com/brainboxdotcc/DPP.git
    GIT_TAG        master     # Or pin a stable release tag, e.g. "v10.0.12"
    GIT_SHALLOW    TRUE
)
FetchContent_MakeAvailable(libdpp)

# -------------------------------------------------------------------------
# 6) Gather source files from the "src/" directory (recursively).
#    从 "src/" 目录中递归收集源文件。
file(GLOB_RECURSE SRC_FILES "${CMAKE_SOURCE_DIR}/src/*.cpp")

# -------------------------------------------------------------------------
# 7) Define an executable named "marong" that builds from the collected sources.
#    定义名为 "marong" 的可执行文件，并编译收集到的源文件。
add_executable(${PROJECT_NAME}
    ${SRC_FILES}
)

# -------------------------------------------------------------------------
# 8) Specify include directories for the "marong" target.
#    指定 "marong" 目标的头文件搜索路径。
target_include_directories(${PROJECT_NAME} PUBLIC
    "${CMAKE_SOURCE_DIR}/include"
    ${OPENSSL_INCLUDE_DIR}
    ${CURL_INCLUDE_DIRS}
)

# -------------------------------------------------------------------------
# 9) Link the required libraries to the "marong" executable:
#    - dl
#    - Threads::Threads
#    - OpenSSL
#    - CURL
#    - DPP
#    将所需的库链接到 "marong" 可执行文件：
#    - dl
#    - 线程库
#    - OpenSSL
#    - CURL
#    - DPP
target_link_libraries(${PROJECT_NAME}
    dl
    Threads::Threads
    OpenSSL::SSL
    OpenSSL::Crypto
    ${CURL_LIBRARIES}
    dpp
)

# -------------------------------------------------------------------------
# 10) Enable DPP coroutines (optional).
#     启用 DPP 的协程支持（可选）。
target_compile_definitions(${PROJECT_NAME} PUBLIC DPP_CORO)
