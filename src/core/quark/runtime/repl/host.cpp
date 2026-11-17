// REPL host. One long-lived process owns the GC and the session variables.
// Each input runs in a forked child: the child loads the input's library,
// calls it, and on success becomes the host; the parent exits. A runtime
// error, Ctrl-C or a crash kills only the child, so the session stays as it
// was. Linux and macOS only (there is no fork on Windows).
#include "quark/quark.hpp"
#include "quark/repl/state.hpp"

#include <cstdio>
#include <cstring>
#include <string>
#include <unistd.h>
#include <sys/wait.h>
#include <dlfcn.h>
#include <signal.h>
#include <errno.h>

extern "C" void GC_set_handle_fork(int);

static int result_fd = 3;

static void write_all(int fd, const char* s) {
    size_t n = std::strlen(s);
    while (n > 0) {
        ssize_t w = ::write(fd, s, n);
        if (w < 0) {
            if (errno == EINTR) continue;
            _exit(1);
        }
        s += w;
        n -= static_cast<size_t>(w);
    }
}

static int read_line(std::string& out) {
    out.clear();
    char c;
    for (;;) {
        ssize_t r = ::read(0, &c, 1);
        if (r == 0) return 0;
        if (r < 0) {
            if (errno == EINTR) continue;
            return -1;
        }
        if (c == '\n') return 1;
        out.push_back(c);
    }
}

static void report_signal(int sig) {
    const char* name = strsignal(sig);
    if (name == nullptr) name = "unknown";
    std::fprintf(stderr, "error[QK-REPL-001] (repl): the input stopped with signal %s; the session is as it was before the input\n", name);
}

int main(int argc, char** argv) {
    for (int i = 1; i + 1 < argc; i++) {
        if (std::strcmp(argv[i], "--result-fd") == 0) {
            result_fd = std::atoi(argv[i + 1]);
        }
    }
    GC_set_handle_fork(1);
    // The REPL runs every input on one thread (specs/tables_v0_3_spec.md
    // D52), whatever QUARK_THREADS says.
    q_par_one_thread();
    q_gc_init();
    std::signal(SIGINT, SIG_IGN);

    std::string line;
    while (read_line(line) == 1) {
        if (line == "quit") {
            return 0;
        }
        if (line.compare(0, 4, "run\t") != 0) {
            write_all(result_fd, "fail\n");
            continue;
        }
        std::string rest = line.substr(4);
        auto tab = rest.rfind('\t');
        if (tab == std::string::npos) {
            write_all(result_fd, "fail\n");
            continue;
        }
        std::string path = rest.substr(0, tab);
        std::string symbol = rest.substr(tab + 1);

        std::fflush(stdout);
        std::fflush(stderr);
        int notify[2];
        if (pipe(notify) != 0) {
            write_all(result_fd, "fail\n");
            continue;
        }
        pid_t pid = fork();
        if (pid < 0) {
            close(notify[0]);
            close(notify[1]);
            write_all(result_fd, "fail\n");
            continue;
        }
        if (pid == 0) {
            close(notify[0]);
            std::signal(SIGINT, SIG_DFL);
            void* lib = dlopen(path.c_str(), RTLD_NOW | RTLD_GLOBAL);
            if (lib == nullptr) {
                std::fprintf(stderr, "error[QK-REPL-001] (repl): could not load the input: %s\n", dlerror());
                _exit(1);
            }
            auto fn = reinterpret_cast<void (*)()>(dlsym(lib, symbol.c_str()));
            if (fn == nullptr) {
                std::fprintf(stderr, "error[QK-REPL-001] (repl): could not load the input: %s\n", dlerror());
                _exit(1);
            }
            fn();
            std::fflush(stdout);
            std::fflush(stderr);
            char ok = 1;
            if (::write(notify[1], &ok, 1) != 1) _exit(1);
            close(notify[1]);
            std::signal(SIGINT, SIG_IGN);
            continue;
        }
        close(notify[1]);
        char ok = 0;
        ssize_t n = ::read(notify[0], &ok, 1);
        close(notify[0]);
        if (n == 1 && ok == 1) {
            write_all(result_fd, "ok\n");
            _exit(0);
        }
        int status = 0;
        while (waitpid(pid, &status, 0) < 0 && errno == EINTR) {
        }
        if (WIFSIGNALED(status)) {
            int sig = WTERMSIG(status);
            if (sig == SIGINT) {
                write_all(result_fd, "interrupted\n");
            } else {
                report_signal(sig);
                write_all(result_fd, "signal\n");
            }
        } else {
            write_all(result_fd, "fail\n");
        }
    }
    return 0;
}
