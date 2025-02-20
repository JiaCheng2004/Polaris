#include "utils/token_tracker.hpp"
#include <atomic>

namespace
{
    /**
     * @brief Global atomic integer to keep track of token usage.
     */
    std::atomic<int> g_totalUsage{0};
}

namespace utils
{
void TokenTracker::addUsage(int count)
{
    g_totalUsage += count;
}

int TokenTracker::getTotalUsage()
{
    return g_totalUsage.load();
}

} // namespace utils
