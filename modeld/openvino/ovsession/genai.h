#ifndef CONTENOX_OV_GENAI_H
#define CONTENOX_OV_GENAI_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct cx_genai_session cx_genai_session;
typedef struct cx_genai_stream cx_genai_stream;

typedef struct cx_ov_device_info {
    int index;
    char name[128];
    char description[256];
    char type[32];
    uint64_t memory_free;
    uint64_t memory_total;
    int memory_free_known;
    int memory_total_known;
    int shared_with_display;
} cx_ov_device_info;

typedef struct cx_ov_runtime_info {
    char runtime_name[64];
    char runtime_digest[128];
    char system_info[512];
    int supports_gpu_offload;
    size_t device_count;
    cx_ov_device_info devices[16];
} cx_ov_runtime_info;

typedef struct cx_genai_metrics {
    size_t requests;
    size_t scheduled_requests;
    float  cache_usage;
    float  max_cache_usage;
    float  avg_cache_usage;
    float  inference_duration;
    size_t cache_size_in_bytes;
} cx_genai_metrics;

/* One dynamic LoRA adapter applied to a GenAI session. Path points at an
   OpenVINO safetensors adapter file (NOT a GGUF adapter — that is the llama
   backend's format). alpha is the effective LoRA scale; OpenVINO already folds
   rank normalization (alpha/rank) and any user weight into this single value. */
typedef struct cx_genai_lora_adapter {
    const char *path;
    float alpha;
} cx_genai_lora_adapter;

typedef struct cx_genai_session_config {
    const char *kv_cache_precision;
    size_t cache_size;
    int dynamic_split_fuse;
    int enable_prefix_caching;
    int use_sparse_attention;
    size_t num_last_dense_tokens_in_prefill;
    float xattention_threshold;
    size_t xattention_block_size;
    size_t xattention_stride;
    /* Native KV cache eviction (sink + recent + evictable middle). When
       use_cache_eviction is set, the residency policy is enforced by OpenVINO's
       own CacheEvictionConfig instead of by runtime KV surgery. Sizes are in
       tokens; max must exceed start + recent. */
    int use_cache_eviction;
    size_t cache_evict_start_size;
    size_t cache_evict_recent_size;
    size_t cache_evict_max_size;
    /* Dynamic LoRA adapters registered on this session's pipeline (MODE_DYNAMIC)
       and activated in the default generation config. lora_adapters points at an
       array of lora_adapter_count entries; a count of 0 means the base model with
       no adapter overhead. The caller owns the array and its path strings; they
       only need to outlive cx_genai_session_new (adapters are loaded eagerly). */
    const cx_genai_lora_adapter *lora_adapters;
    size_t lora_adapter_count;
} cx_genai_session_config;

int cx_ov_runtime_info_get(cx_ov_runtime_info *out, char *err, size_t err_len);
int cx_ov_device_info_get(const char *device, cx_ov_device_info *out, char *err, size_t err_len);

cx_genai_session *cx_genai_session_new(const char *model_dir, const char *device,
                                       const cx_genai_session_config *config,
                                       char *err, size_t err_len);
void cx_genai_session_free(cx_genai_session *s);
int cx_genai_session_cancel(cx_genai_session *s);

int cx_genai_apply_chat_template(cx_genai_session *s,
                                 const char **roles,
                                 const char **contents,
                                 const char **tool_calls,
                                 const char **tool_call_ids,
                                 size_t n,
                                 const char *tools_json,
                                 int add_generation_prompt,
                                 char **out,
                                 size_t *out_len,
                                 char *err,
                                 size_t err_len);

int cx_genai_tokenize(cx_genai_session *s,
                      const char *prompt,
                      int add_special_tokens,
                      int64_t *tokens,
                      size_t tokens_len,
                      size_t *tokens_out,
                      char *err,
                      size_t err_len);

int cx_genai_supports_cold_kv(cx_genai_session *s);
int cx_genai_export_cold_kv(cx_genai_session *s,
                            int start,
                            int end,
                            const int64_t *tokens,
                            size_t tokens_len,
                            const int64_t *prefix_tokens,
                            size_t prefix_tokens_len,
                            const char *token_hash,
                            uint8_t **out,
                            size_t *out_len,
                            char *err,
                            size_t err_len);
int cx_genai_import_cold_kv(cx_genai_session *s,
                            int start,
                            int end,
                            int dest_start,
                            const int64_t *tokens,
                            size_t tokens_len,
                            const int64_t *prefix_tokens,
                            size_t prefix_tokens_len,
                            const char *token_hash,
                            const uint8_t *data,
                            size_t data_len,
                            char *err,
                            size_t err_len);
void cx_genai_kv_data_free(void *p);
void cx_genai_data_free(void *p);

int cx_genai_generate(cx_genai_session *s,
                      const char *prompt,
                      size_t max_new_tokens,
                      float temperature,
                      int use_temperature,
                      float top_p,
                      int use_top_p,
                      size_t top_k,
                      int use_top_k,
                      size_t seed,
                      int use_seed,
                      const char *structured_protocol,
                      const char *structured_payload,
                      const char *parser_protocols,
                      char **out,
                      size_t *out_len,
                      char **parsed,
                      size_t *parsed_len,
                      cx_genai_metrics *metrics,
                      char *err,
                      size_t err_len);

int cx_genai_prefill_tokens(cx_genai_session *s,
                            const int64_t *tokens,
                            size_t tokens_len,
                            cx_genai_metrics *metrics,
                            char *err,
                            size_t err_len);

int cx_genai_generate_tokens(cx_genai_session *s,
                             const int64_t *tokens,
                             size_t tokens_len,
                             size_t max_new_tokens,
                             float temperature,
                             int use_temperature,
                             float top_p,
                             int use_top_p,
                             size_t top_k,
                             int use_top_k,
                             size_t seed,
                             int use_seed,
                             const char *structured_protocol,
                             const char *structured_payload,
                             const char *parser_protocols,
                             char **out,
                             size_t *out_len,
                             char **parsed,
                             size_t *parsed_len,
                             cx_genai_metrics *metrics,
                             char *err,
                             size_t err_len);

cx_genai_stream *cx_genai_stream_new(void);
void cx_genai_stream_free(cx_genai_stream *stream);
void cx_genai_stream_abort(cx_genai_stream *stream, const char *message);
int cx_genai_stream_next(cx_genai_stream *stream,
                         char **out,
                         size_t *out_len,
                         char **thinking,
                         size_t *thinking_len,
                         char *err,
                         size_t err_len);

int cx_genai_generate_stream(cx_genai_session *s,
                             const char *prompt,
                             size_t max_new_tokens,
                             float temperature,
                             int use_temperature,
                             float top_p,
                             int use_top_p,
                             size_t top_k,
                             int use_top_k,
                             size_t seed,
                             int use_seed,
                             const char *parser_protocols,
                             cx_genai_stream *stream,
                             cx_genai_metrics *metrics,
                             char *err,
                             size_t err_len);

int cx_genai_generate_tokens_stream(cx_genai_session *s,
                                    const int64_t *tokens,
                                    size_t tokens_len,
                                    size_t max_new_tokens,
                                    float temperature,
                                    int use_temperature,
                                    float top_p,
                                    int use_top_p,
                                    size_t top_k,
                                    int use_top_k,
                                    size_t seed,
                                    int use_seed,
                                    const char *parser_protocols,
                                    cx_genai_stream *stream,
                                    cx_genai_metrics *metrics,
                                    char *err,
                                    size_t err_len);

#ifdef __cplusplus
}
#endif

#endif
