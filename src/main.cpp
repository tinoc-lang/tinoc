#include <cstdlib>
#include <iostream>

int main(int argc, char** argv) {
    if (argc < 2) {
        std::cerr << "usage: tinoc <source-file>\n";
        return EXIT_FAILURE;
    }

    std::cout << "tinoc: transpiling " << argv[1] << "\n";
    return EXIT_SUCCESS;
}
