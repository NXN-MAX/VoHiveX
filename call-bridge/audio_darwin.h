#include <stdint.h>
int vx_audio_start(uint32_t location);
void vx_audio_stop(void);
int vx_audio_read(int16_t *dst, int capacity);
int vx_audio_write(const int16_t *src, int count);
