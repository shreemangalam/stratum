#include <stdio.h>
#include <string.h>

typedef struct {
    int x;
    int y;
    int z;
    char label[32];
} Point;

void print_point(Point p) {
    printf("(%d, %d, %d) %s\n", p.x, p.y, p.z, p.label);
}
