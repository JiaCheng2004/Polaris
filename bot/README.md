# Marong Bot

A Discord bot built using [D++ (DPP)](https://dpp.dev/) in C++20.

## Project Structure

- **src/main.cpp** — The entry point of the bot.
- **src/commands**, **src/events** — Subdirectories for commands, event handlers, etc.
- **include/commands**, **include/events** — Corresponding header files.
- **config/config.json** — Bot configuration (token, prefix, etc.).

## Building Locally

```bash
# Inside the 'bot' folder
mkdir build && cd build
cmake ..
cmake --build .
./marong
