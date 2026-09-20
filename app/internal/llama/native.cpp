//go:build cgo && peanut_llama

#include "native.h"

#include "llama.h"
#include "json-schema-to-grammar.h"
#include "json.h"

#include <atomic>
#include <new>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>
#include <string>

#define PEANUT_LLAMA_CONTEXT_SIZE 4096
#define PEANUT_LLAMA_BATCH_SIZE 2048
#define PEANUT_LLAMA_MAX_OUTPUT_BYTES (4u * 1024u * 1024u)
#define PEANUT_LLAMA_SAMPLER_SEED 42u

struct peanut_llama {
	struct llama_model *model;
	struct llama_context *context;
	struct llama_sampler *sampler;
	char *output;
	size_t output_len;
	std::atomic_bool abort_requested;
};

static void peanut_log(enum ggml_log_level level, const char *text, void *user_data) {
	(void) level;
	(void) text;
	(void) user_data;
}

static bool peanut_abort_callback(void *user_data) {
	return static_cast<std::atomic_bool *>(user_data)->load(std::memory_order_relaxed);
}

void peanut_llama_backend_init(void) {
	llama_log_set(peanut_log, NULL);
	llama_backend_init();
}

static void peanut_free_sampler(struct peanut_llama *engine) {
	if (engine != NULL && engine->sampler != NULL) {
		llama_sampler_free(engine->sampler);
		engine->sampler = NULL;
	}
}

void peanut_llama_free_output(peanut_llama *engine) {
	if (engine == NULL) {
		return;
	}
	free(engine->output);
	engine->output = NULL;
	engine->output_len = 0;
}

static bool peanut_schema_to_grammar(const char *schema, std::string *grammar) {
	try {
		*grammar = json_schema_to_grammar(common_json::parse(schema), true);
		return !grammar->empty();
	} catch (...) {
		return false;
	}
}

peanut_llama_status peanut_llama_open(const char *model_path, int32_t threads, peanut_llama **out) {
	if (model_path == NULL || model_path[0] == '\0' || threads <= 0 || out == NULL) {
		return PEANUT_LLAMA_INVALID;
	}
	*out = NULL;

	struct peanut_llama *engine = new (std::nothrow) peanut_llama{};
	if (engine == NULL) {
		return PEANUT_LLAMA_CONTEXT_FAILED;
	}
	engine->abort_requested.store(false, std::memory_order_relaxed);

	struct llama_model_params model_params = llama_model_default_params();
	model_params.n_gpu_layers = 0;
	engine->model = llama_model_load_from_file(model_path, model_params);
	if (engine->model == NULL) {
		delete engine;
		return PEANUT_LLAMA_LOAD_FAILED;
	}

	struct llama_context_params context_params = llama_context_default_params();
	context_params.n_ctx = PEANUT_LLAMA_CONTEXT_SIZE;
	context_params.n_batch = PEANUT_LLAMA_BATCH_SIZE;
	context_params.n_ubatch = PEANUT_LLAMA_BATCH_SIZE;
	context_params.n_threads = threads;
	context_params.n_threads_batch = threads;
	context_params.abort_callback = peanut_abort_callback;
	context_params.abort_callback_data = &engine->abort_requested;
	engine->context = llama_init_from_model(engine->model, context_params);
	if (engine->context == NULL) {
		llama_model_free(engine->model);
		delete engine;
		return PEANUT_LLAMA_CONTEXT_FAILED;
	}

	*out = engine;
	return PEANUT_LLAMA_OK;
}

static peanut_llama_status peanut_append_piece(struct peanut_llama *engine, const struct llama_vocab *vocab, llama_token token) {
	char stack_buffer[256];
	int32_t piece_len = llama_token_to_piece(vocab, token, stack_buffer, sizeof(stack_buffer), 0, true);
	if (piece_len == 0) {
		return PEANUT_LLAMA_OK;
	}
	if (piece_len < 0) {
		int32_t required = -piece_len;
		char *buffer = static_cast<char *>(malloc((size_t) required));
		if (buffer == NULL) {
			return PEANUT_LLAMA_OUTPUT_FAILED;
		}
		piece_len = llama_token_to_piece(vocab, token, buffer, required, 0, true);
		if (piece_len <= 0) {
			free(buffer);
			return PEANUT_LLAMA_OUTPUT_FAILED;
		}
		if (engine->output_len + (size_t) piece_len > PEANUT_LLAMA_MAX_OUTPUT_BYTES) {
			free(buffer);
			return PEANUT_LLAMA_OUTPUT_FAILED;
		}
		char *grown = static_cast<char *>(realloc(engine->output, engine->output_len + (size_t) piece_len + 1));
		if (grown == NULL) {
			free(buffer);
			return PEANUT_LLAMA_OUTPUT_FAILED;
		}
		engine->output = grown;
		memcpy(engine->output + engine->output_len, buffer, (size_t) piece_len);
		engine->output_len += (size_t) piece_len;
		engine->output[engine->output_len] = '\0';
		free(buffer);
		return PEANUT_LLAMA_OK;
	}
	if (engine->output_len + (size_t) piece_len > PEANUT_LLAMA_MAX_OUTPUT_BYTES) {
		return PEANUT_LLAMA_OUTPUT_FAILED;
	}
	char *grown = static_cast<char *>(realloc(engine->output, engine->output_len + (size_t) piece_len + 1));
	if (grown == NULL) {
		return PEANUT_LLAMA_OUTPUT_FAILED;
	}
	engine->output = grown;
	memcpy(engine->output + engine->output_len, stack_buffer, (size_t) piece_len);
	engine->output_len += (size_t) piece_len;
	engine->output[engine->output_len] = '\0';
	return PEANUT_LLAMA_OK;
}

peanut_llama_status peanut_llama_generate(
	peanut_llama *engine,
	const char *prompt,
	const char *schema,
	int32_t max_tokens,
	const unsigned char **output,
	size_t *output_len) {
	if (engine == NULL || prompt == NULL || prompt[0] == '\0' || schema == NULL || max_tokens <= 0 ||
		output == NULL || output_len == NULL) {
		return PEANUT_LLAMA_INVALID;
	}
	*output = NULL;
	*output_len = 0;
	peanut_llama_free_output(engine);
	engine->abort_requested.store(false, std::memory_order_relaxed);
	if (engine->context == NULL || llama_get_memory(engine->context) == NULL) {
		return PEANUT_LLAMA_CONTEXT_FAILED;
	}
	llama_memory_clear(llama_get_memory(engine->context), true);

	const struct llama_vocab *vocab = llama_model_get_vocab(engine->model);
	int32_t prompt_tokens_len = llama_tokenize(vocab, prompt, (int32_t) strlen(prompt), NULL, 0, true, true);
	if (prompt_tokens_len >= 0) {
		return PEANUT_LLAMA_TOKENIZE_FAILED;
	}
	prompt_tokens_len = -prompt_tokens_len;
	llama_token *prompt_tokens = static_cast<llama_token *>(malloc((size_t) prompt_tokens_len * sizeof(*prompt_tokens)));
	if (prompt_tokens == NULL) {
		return PEANUT_LLAMA_TOKENIZE_FAILED;
	}
	if (llama_tokenize(vocab, prompt, (int32_t) strlen(prompt), prompt_tokens, prompt_tokens_len, true, true) < 0) {
		free(prompt_tokens);
		return PEANUT_LLAMA_TOKENIZE_FAILED;
	}

	std::string grammar;
	if (!peanut_schema_to_grammar(schema, &grammar)) {
		free(prompt_tokens);
		return PEANUT_LLAMA_GRAMMAR_FAILED;
	}

	struct llama_sampler_chain_params sampler_params = llama_sampler_chain_default_params();
	engine->sampler = llama_sampler_chain_init(sampler_params);
	if (engine->sampler == NULL) {
		free(prompt_tokens);
		return PEANUT_LLAMA_GRAMMAR_FAILED;
	}
	struct llama_sampler *grammar_sampler = llama_sampler_init_grammar(vocab, grammar.c_str(), "root");
	if (grammar_sampler == NULL) {
		free(prompt_tokens);
		peanut_free_sampler(engine);
		return PEANUT_LLAMA_GRAMMAR_FAILED;
	}
	llama_sampler_chain_add(engine->sampler, grammar_sampler);
	llama_sampler_chain_add(engine->sampler, llama_sampler_init_temp(0.0f));
	llama_sampler_chain_add(engine->sampler, llama_sampler_init_dist(PEANUT_LLAMA_SAMPLER_SEED));

	struct llama_batch batch = llama_batch_get_one(prompt_tokens, prompt_tokens_len);
	if (llama_model_has_encoder(engine->model)) {
		if (llama_encode(engine->context, batch) != 0) {
			free(prompt_tokens);
			peanut_free_sampler(engine);
			return engine->abort_requested.load(std::memory_order_relaxed) ? PEANUT_LLAMA_CANCELLED : PEANUT_LLAMA_DECODE_FAILED;
		}
		llama_token decoder_start = llama_model_decoder_start_token(engine->model);
		if (decoder_start == LLAMA_TOKEN_NULL) {
			decoder_start = llama_vocab_bos(vocab);
		}
		batch = llama_batch_get_one(&decoder_start, 1);
	}

	for (int32_t generated = 0; generated < max_tokens; generated++) {
		int32_t decode_status = llama_decode(engine->context, batch);
		if (decode_status != 0) {
			free(prompt_tokens);
			peanut_free_sampler(engine);
			return engine->abort_requested.load(std::memory_order_relaxed) || decode_status == 2 ? PEANUT_LLAMA_CANCELLED : PEANUT_LLAMA_DECODE_FAILED;
		}
		llama_token token = llama_sampler_sample(engine->sampler, engine->context, -1);
		if (llama_vocab_is_eog(vocab, token)) {
			break;
		}
		peanut_llama_status append_status = peanut_append_piece(engine, vocab, token);
		if (append_status != PEANUT_LLAMA_OK) {
			free(prompt_tokens);
			peanut_free_sampler(engine);
			return append_status;
		}
		batch = llama_batch_get_one(&token, 1);
	}

	free(prompt_tokens);
	peanut_free_sampler(engine);
	if (engine->abort_requested.load(std::memory_order_relaxed)) {
		return PEANUT_LLAMA_CANCELLED;
	}
	if (engine->output == NULL) {
		return PEANUT_LLAMA_OUTPUT_FAILED;
	}
	*output = (const unsigned char *) engine->output;
	*output_len = engine->output_len;
	return PEANUT_LLAMA_OK;
}

void peanut_llama_abort(peanut_llama *engine) {
	if (engine != NULL) {
		engine->abort_requested.store(true, std::memory_order_relaxed);
	}
}

void peanut_llama_close(peanut_llama *engine) {
	if (engine == NULL) {
		return;
	}
	peanut_free_sampler(engine);
	peanut_llama_free_output(engine);
	if (engine->context != NULL) {
		llama_free(engine->context);
	}
	if (engine->model != NULL) {
		llama_model_free(engine->model);
	}
	delete engine;
}
