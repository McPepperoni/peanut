#ifndef PEANUT_LLAMA_NATIVE_H
#define PEANUT_LLAMA_NATIVE_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct peanut_llama peanut_llama;

typedef enum peanut_llama_status {
	PEANUT_LLAMA_OK = 0,
	PEANUT_LLAMA_INVALID = 1,
	PEANUT_LLAMA_LOAD_FAILED = 2,
	PEANUT_LLAMA_CONTEXT_FAILED = 3,
	PEANUT_LLAMA_CHAT_TEMPLATE_FAILED = 4,
	PEANUT_LLAMA_GRAMMAR_FAILED = 5,
	PEANUT_LLAMA_TOKENIZE_FAILED = 6,
	PEANUT_LLAMA_DECODE_FAILED = 7,
	PEANUT_LLAMA_CANCELLED = 8,
	PEANUT_LLAMA_OUTPUT_FAILED = 9
} peanut_llama_status;

void peanut_llama_backend_init(void);
peanut_llama_status peanut_llama_open(const char *model_path, int32_t threads, peanut_llama **out);
peanut_llama_status peanut_llama_generate(
	peanut_llama *engine,
	const char *prompt,
	const char *schema,
	int32_t max_tokens,
	const unsigned char **output,
	size_t *output_len);
void peanut_llama_abort(peanut_llama *engine);
void peanut_llama_free_output(peanut_llama *engine);
void peanut_llama_close(peanut_llama *engine);

#ifdef __cplusplus
}
#endif

#endif
