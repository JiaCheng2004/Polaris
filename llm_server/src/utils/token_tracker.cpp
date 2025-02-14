#include "utils/token_tracker.hpp"
#include <atomic>

namespace {
    // A simple atomic for demonstration
    std::atomic<int> g_totalUsage{0};
}

namespace utils {

void TokenTracker::addUsage(int count) {
    g_totalUsage += count;
}

int TokenTracker::getTotalUsage() {
    return g_totalUsage.load();
}

} // namespace utils
