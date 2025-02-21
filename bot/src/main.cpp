#include <dpp/dpp.h>
#include <cstdlib>
#include <iostream>

int main(int argc, char const *argv[])
{
    // Retrieve the bot token from the environment variable
    const char* token = std::getenv("BOT_TOKEN");
    if (!token) {
        std::cerr << "Error: BOT_TOKEN environment variable not set!" << std::endl;
        return 1;
    }

    /* Setup the bot using the token from the environment */
    dpp::cluster bot(token);

    /* Output simple log messages to stdout */
    bot.on_log(dpp::utility::cout_logger());

    /* Handle slash command */
    bot.on_slashcommand([](const dpp::slashcommand_t& event) {
         if (event.command.get_command_name() == "ping") {
            event.reply("Pong!");
         }
    });

    /* Register slash command on_ready */
    bot.on_ready([&bot](const dpp::ready_t& event) {
        if (dpp::run_once<struct register_bot_commands>()) {
            bot.global_command_create(dpp::slashcommand("ping", "ping pong testing!", bot.me.id));
        }
    });

    /* Start the bot */
    bot.start(dpp::st_wait);

    return 0;
}
