/* Test-only probe for the pinned ICU C ABI, not an application adapter. */
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

extern void udata_setFileAccess_78(int32_t, int32_t *);
extern void *uidna_openUTS46_78(uint32_t, int32_t *);
extern void uidna_close_78(void *);
extern int32_t uidna_nameToASCII_UTF8_78(const void *, const char *, int32_t,
                                      char *, int32_t, void *, int32_t *);
struct info { int16_t size; uint8_t transitional, reserved; uint32_t errors; int32_t a, b; };

int main(void) {
    int32_t error = 0;
    udata_setFileAccess_78(3, &error);
    void *idna = uidna_openUTS46_78(30, &error);
    if (error > 0 || !idna) return 4;
    char *line = NULL;
    size_t capacity = 0;
    ssize_t size;
    while ((size = getline(&line, &capacity, stdin)) >= 0) {
        if (size && line[size - 1] == '\n') size--;
        if (size % 2 || size > 1048576) return 2;
        unsigned char *input = malloc((size_t)size / 2 + 1);
        if (!input) return 5;
        for (ssize_t i = 0; i < size; i += 2) {
            unsigned int value;
            if (sscanf(line + i, "%2x", &value) != 1) return 3;
            input[i / 2] = (unsigned char)value;
        }
        char output[256];
        struct info info = {sizeof(struct info), 0, 0, 0, 0, 0};
        error = 0;
        int32_t length = uidna_nameToASCII_UTF8_78(idna, (char *)input, (int32_t)(size / 2),
                                                  output, sizeof(output), &info, &error);
        free(input);
        /* Endpoint policy separately removes/restores one terminal root dot. */
        if (error > 0 || info.errors || length <= 0 || output[length - 1] == '.') {
            puts("error");
        } else {
            fputs("ok:", stdout);
            for (int32_t i = 0; i < length; i++) printf("%02x", (unsigned char)output[i]);
            putchar('\n');
        }
    }
    free(line);
    uidna_close_78(idna);
    return 0;
}
