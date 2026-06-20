//go:build openvino && openvino_genai

#include "genai.h"

#include <atomic>
#include <algorithm>
#include <cctype>
#include <condition_variable>
#include <cstdint>
#include <cstring>
#include <deque>
#include <exception>
#include <filesystem>
#include <functional>
#include <memory>
#include <mutex>
#include <sstream>
#include <stdexcept>
#include <string>
#include <thread>
#include <utility>
#include <vector>

#include <openvino/core/version.hpp>
#include <openvino/openvino.hpp>
#include <openvino/runtime/intel_gpu/properties.hpp>
#include <openvino/runtime/intel_npu/properties.hpp>
#include <openvino/runtime/properties.hpp>
#include <openvino/genai/continuous_batching_pipeline.hpp>
#include <openvino/genai/generation_config.hpp>
#include <openvino/genai/parsers.hpp>
#include <openvino/genai/scheduler_config.hpp>
#include <openvino/genai/sparse_attention.hpp>
#include <openvino/genai/streamer_base.hpp>
#include <openvino/genai/chat_history.hpp>
#include <openvino/genai/tokenizer.hpp>
#include <openvino/genai/json_container.hpp>

static void write_buf(char *dst, size_t dst_len, const std::string &value) {
    if (!dst || dst_len == 0) return;
    std::strncpy(dst, value.c_str(), dst_len - 1);
    dst[dst_len - 1] = '\0';
}

template <size_t N>
static void write_fixed(char (&dst)[N], const std::string &value) {
    write_buf(dst, N, value);
}

static std::string upper_device_base(const std::string &device) {
    auto base = device.substr(0, device.find('.'));
    std::transform(base.begin(), base.end(), base.begin(), [](unsigned char c) {
        return static_cast<char>(std::toupper(c));
    });
    return base;
}

static bool is_accelerator_device(const std::string &device) {
    const auto base = upper_device_base(device);
    return base == "GPU" || base == "NPU";
}

static std::string select_available_device(const std::vector<std::string> &devices, const std::string &requested) {
    if (requested.empty()) {
        return "CPU";
    }
    for (const auto &dev : devices) {
        if (dev == requested) {
            return dev;
        }
    }
    const auto req_base = upper_device_base(requested);
    for (const auto &dev : devices) {
        if (upper_device_base(dev) == req_base) {
            return dev;
        }
    }
    return requested;
}

static void fill_device_info(ov::Core &core, const std::string &device, int index, cx_ov_device_info *out) {
    if (!out) return;
    std::memset(out, 0, sizeof(*out));
    out->index = index;
    write_fixed(out->name, device);

    const auto base = upper_device_base(device);
    std::string type = base;
    std::string description;
    bool shared = false;

    try {
        description = core.get_property(device, ov::device::full_name);
    } catch (...) {
        description = device;
    }

    try {
        const auto dev_type = core.get_property(device, ov::device::type);
        if (dev_type == ov::device::Type::INTEGRATED) {
            shared = true;
            if (!description.empty()) {
                description += " (integrated)";
            }
        } else if (dev_type == ov::device::Type::DISCRETE && !description.empty()) {
            description += " (discrete)";
        }
    } catch (...) {
    }

    if (base == "GPU") {
        type = shared ? "igpu" : "gpu";
        try {
            out->memory_total = core.get_property(device, ov::intel_gpu::device_total_mem_size);
        } catch (...) {
        }
        try {
            auto free = core.get_property(device, ov::intel_gpu::hint::available_device_mem);
            if (free > 0) {
                out->memory_free = static_cast<uint64_t>(free);
            }
        } catch (...) {
        }
    } else if (base == "NPU") {
        type = "accel";
        try {
            out->memory_total = core.get_property(device, ov::intel_npu::device_total_mem_size);
        } catch (...) {
        }
        try {
            auto allocated = core.get_property(device, ov::intel_npu::device_alloc_mem_size);
            if (out->memory_total > allocated) {
                out->memory_free = out->memory_total - allocated;
            }
        } catch (...) {
            out->memory_free = out->memory_total;
        }
    } else if (base == "CPU") {
        type = "cpu";
    }

    if (out->memory_free == 0 && out->memory_total > 0) {
        out->memory_free = out->memory_total;
    }
    out->shared_with_display = shared ? 1 : 0;
    write_fixed(out->description, description);
    write_fixed(out->type, type);
}

extern "C" int cx_ov_runtime_info_get(cx_ov_runtime_info *out, char *err, size_t err_len) {
    try {
        if (!out) {
            throw std::runtime_error("runtime info output pointer is null");
        }
        std::memset(out, 0, sizeof(*out));
        write_fixed(out->runtime_name, "OpenVINO GenAI");

        const auto version = ov::get_openvino_version();
        std::string digest = version.buildNumber ? version.buildNumber : "";
        std::string desc = version.description ? version.description : "";
        write_fixed(out->runtime_digest, digest);
        write_fixed(out->system_info, desc);

        ov::Core core;
        auto devices = core.get_available_devices();
        out->device_count = std::min(devices.size(), static_cast<size_t>(16));
        for (size_t i = 0; i < out->device_count; ++i) {
            fill_device_info(core, devices[i], static_cast<int>(i), &out->devices[i]);
            if (is_accelerator_device(devices[i])) {
                out->supports_gpu_offload = 1;
            }
        }
        return 0;
    } catch (const std::exception &e) {
        write_buf(err, err_len, e.what());
        return 1;
    } catch (...) {
        write_buf(err, err_len, "unknown OpenVINO runtime info error");
        return 1;
    }
}

extern "C" int cx_ov_device_info_get(const char *device, cx_ov_device_info *out, char *err, size_t err_len) {
    try {
        if (!out) {
            throw std::runtime_error("device info output pointer is null");
        }
        ov::Core core;
        auto devices = core.get_available_devices();
        const std::string selected = select_available_device(devices, device ? std::string(device) : std::string("CPU"));
        fill_device_info(core, selected, 0, out);
        return 0;
    } catch (const std::exception &e) {
        write_buf(err, err_len, e.what());
        return 1;
    } catch (...) {
        write_buf(err, err_len, "unknown OpenVINO device info error");
        return 1;
    }
}

static ov::genai::SchedulerConfig scheduler_config_from(const cx_genai_session_config *config) {
    ov::genai::SchedulerConfig cfg;
    cfg.cache_size = config ? config->cache_size : 1;
    cfg.dynamic_split_fuse = !config || config->dynamic_split_fuse != 0;
    cfg.enable_prefix_caching = !config || config->enable_prefix_caching != 0;
    cfg.use_sparse_attention = !config || config->use_sparse_attention != 0;
    cfg.sparse_attention_config.mode = ov::genai::SparseAttentionMode::XATTENTION;
    cfg.sparse_attention_config.num_last_dense_tokens_in_prefill =
        config ? config->num_last_dense_tokens_in_prefill : 10;
    cfg.sparse_attention_config.xattention_threshold =
        config ? config->xattention_threshold : 0.9f;
    cfg.sparse_attention_config.xattention_block_size =
        config ? config->xattention_block_size : 128;
    cfg.sparse_attention_config.xattention_stride =
        config ? config->xattention_stride : 16;
    if (config && config->use_cache_eviction != 0) {
        // Native sink + recent + evictable-middle eviction. This is the OpenVINO
        // expression of the residency policy: keep the prefix (sinks) and a
        // recent window hot, evict the middle by attention importance.
        cfg.use_cache_eviction = true;
        cfg.cache_eviction_config = ov::genai::CacheEvictionConfig(
            config->cache_evict_start_size,
            config->cache_evict_recent_size,
            config->cache_evict_max_size,
            ov::genai::AggregationMode::NORM_SUM);
    }
    cfg.validate();
    return cfg;
}

static ov::Any kv_precision_from(const cx_genai_session_config *config) {
    std::string precision = (config && config->kv_cache_precision && config->kv_cache_precision[0])
        ? std::string(config->kv_cache_precision)
        : std::string("f16");
    if (precision == "f16" || precision == "FP16" || precision == "fp16") {
        return ov::element::f16;
    }
    if (precision == "f32" || precision == "FP32" || precision == "fp32") {
        return ov::element::f32;
    }
    if (precision == "u8" || precision == "U8") {
        return ov::element::u8;
    }
    if (precision == "i8" || precision == "I8") {
        return ov::element::i8;
    }
    throw std::runtime_error("unsupported KV_CACHE_PRECISION: " + precision);
}

struct cx_genai_session {
    std::unique_ptr<ov::genai::ContinuousBatchingPipeline> pipe;
    std::atomic<bool> cancel_requested{false};

    std::thread worker;
    std::mutex mu;
    std::condition_variable cv;
    bool stopping = false;
    bool busy = false;
    bool has_task = false;
    bool done = false;
    std::function<void()> task;
    std::exception_ptr task_error;

    cx_genai_session() : worker([this] { loop(); }) {}

    ~cx_genai_session() {
        shutdown();
    }

    void run(std::function<void()> fn) {
        std::unique_lock<std::mutex> lock(mu);
        cv.wait(lock, [this] { return (!busy && !has_task) || stopping; });
        if (stopping) {
            throw std::runtime_error("OpenVINO GenAI session is closing");
        }

        busy = true;
        done = false;
        task_error = nullptr;
        task = std::move(fn);
        has_task = true;
        cv.notify_all();

        cv.wait(lock, [this] { return done; });
        busy = false;
        cv.notify_all();

        if (task_error) {
            std::rethrow_exception(task_error);
        }
    }

    void shutdown() {
        if (!worker.joinable()) {
            return;
        }

        try {
            run([this] { pipe.reset(); });
        } catch (...) {
            // Destructors must not throw. The caller-facing free path is best-effort.
        }

        {
            std::lock_guard<std::mutex> lock(mu);
            stopping = true;
            cv.notify_all();
        }
        worker.join();
    }

    void loop() {
        for (;;) {
            std::function<void()> fn;
            {
                std::unique_lock<std::mutex> lock(mu);
                cv.wait(lock, [this] { return has_task || stopping; });
                if (stopping && !has_task) {
                    return;
                }
                fn = std::move(task);
                task = nullptr;
                has_task = false;
            }

            std::exception_ptr err;
            try {
                fn();
            } catch (...) {
                err = std::current_exception();
            }

            {
                std::lock_guard<std::mutex> lock(mu);
                task_error = err;
                done = true;
                cv.notify_all();
            }
        }
    }
};

struct cx_genai_stream_chunk {
    std::string text;
    std::string thinking;
};

struct cx_genai_stream {
    std::mutex mu;
    std::condition_variable cv;
    std::deque<cx_genai_stream_chunk> chunks;
    bool done = false;
    int rc = 0;
    std::string error;

    void push(const std::string &text, const std::string &thinking) {
        if (text.empty() && thinking.empty()) return;
        std::lock_guard<std::mutex> lock(mu);
        chunks.push_back(cx_genai_stream_chunk{text, thinking});
        cv.notify_all();
    }

    void finish(int code, const std::string &message = "") {
        std::lock_guard<std::mutex> lock(mu);
        rc = code;
        error = message;
        done = true;
        cv.notify_all();
    }
};

// Seed from the model's own generation config (eos_token_id, sampling defaults,
// repetition_penalty from generation_config.json) so model behavior is honored;
// override only the fields the caller specifies. apply_chat_template stays false
// because the prompt is already templated with the model's own template upstream.
static ov::genai::GenerationConfig generation_config_from(ov::genai::GenerationConfig gen,
                                                          size_t max_new_tokens,
                                                          float temperature,
                                                          int use_temperature,
                                                          float top_p,
                                                          int use_top_p) {
    if (max_new_tokens > 0) {
        gen.max_new_tokens = max_new_tokens;
    } else if (gen.max_new_tokens == 0 || gen.max_new_tokens == SIZE_MAX) {
        gen.max_new_tokens = 256;
    }
    gen.apply_chat_template = false;
    if (use_temperature) {
        gen.temperature = temperature;
        gen.do_sample = temperature > 0.0f;
    }
    if (use_top_p) {
        gen.top_p = top_p;
        gen.do_sample = true;
    }
    gen.validate();
    return gen;
}

static void copy_metrics(const ov::genai::PipelineMetrics &src, cx_genai_metrics *dst) {
    if (!dst) return;
    dst->requests = src.requests;
    dst->scheduled_requests = src.scheduled_requests;
    dst->cache_usage = src.cache_usage;
    dst->max_cache_usage = src.max_cache_usage;
    dst->avg_cache_usage = src.avg_cache_usage;
    dst->inference_duration = src.inference_duration;
    dst->cache_size_in_bytes = src.cache_size_in_bytes;
}

static std::vector<std::string> split_protocols(const char *value) {
    std::vector<std::string> out;
    if (!value || !value[0]) {
        return out;
    }

    std::stringstream ss{std::string(value)};
    std::string item;
    while (std::getline(ss, item)) {
        while (!item.empty() && (item.back() == '\r' || item.back() == '\n' || item.back() == ' ' || item.back() == '\t')) {
            item.pop_back();
        }
        size_t start = 0;
        while (start < item.size() && (item[start] == ' ' || item[start] == '\t')) {
            start++;
        }
        if (start > 0) {
            item.erase(0, start);
        }
        if (!item.empty()) {
            out.push_back(item);
        }
    }
    return out;
}

static std::shared_ptr<ov::genai::Parser> parser_for_protocol(const std::string &protocol) {
    if (protocol == "openvino:llama3_pythonic_tool_parser") {
        return std::make_shared<ov::genai::Llama3PythonicToolParser>();
    }
    if (protocol == "openvino:llama3_json_tool_parser") {
        return std::make_shared<ov::genai::Llama3JsonToolParser>();
    }
    if (protocol == "openvino:reasoning_parser") {
        return std::make_shared<ov::genai::ReasoningParser>(/*expect_open_tag=*/true, /*keep_original_content=*/false);
    }
    if (protocol == "openvino:deepseek_r1_reasoning_parser") {
        return std::make_shared<ov::genai::ReasoningParser>(/*expect_open_tag=*/false, /*keep_original_content=*/false);
    }
    if (protocol == "openvino:phi4_reasoning_parser") {
        return std::make_shared<ov::genai::ReasoningParser>(/*expect_open_tag=*/true, /*keep_original_content=*/false);
    }
    if (protocol == "openvino:vllm_parser_wrapper") {
        throw std::runtime_error("openvino:vllm_parser_wrapper is a Python OpenVINO GenAI binding and is not available in the native C++ session bridge");
    }
    if (protocol == "openvino:reasoning_incremental_parser" ||
        protocol == "openvino:deepseek_r1_reasoning_incremental_parser" ||
        protocol == "openvino:phi4_reasoning_incremental_parser") {
        throw std::runtime_error(protocol + " requires the streaming parser bridge; non-stream chat generation accepts complete-output Parser protocols only");
    }
    throw std::runtime_error("unsupported OpenVINO parser protocol: " + protocol);
}

static std::shared_ptr<ov::genai::IncrementalParser> incremental_parser_for_protocol(const std::string &protocol) {
    if (protocol == "openvino:reasoning_incremental_parser") {
        return std::make_shared<ov::genai::ReasoningIncrementalParser>(/*expect_open_tag=*/true, /*keep_original_content=*/false);
    }
    if (protocol == "openvino:deepseek_r1_reasoning_incremental_parser") {
        return std::make_shared<ov::genai::ReasoningIncrementalParser>(/*expect_open_tag=*/false, /*keep_original_content=*/false);
    }
    if (protocol == "openvino:phi4_reasoning_incremental_parser") {
        return std::make_shared<ov::genai::ReasoningIncrementalParser>(/*expect_open_tag=*/true, /*keep_original_content=*/false);
    }
    if (protocol == "openvino:llama3_pythonic_tool_parser" ||
        protocol == "openvino:llama3_json_tool_parser" ||
        protocol == "openvino:reasoning_parser" ||
        protocol == "openvino:deepseek_r1_reasoning_parser" ||
        protocol == "openvino:phi4_reasoning_parser") {
        throw std::runtime_error(protocol + " requires the complete-output parser bridge; stream generation accepts incremental Parser protocols only");
    }
    if (protocol == "openvino:vllm_parser_wrapper") {
        throw std::runtime_error("openvino:vllm_parser_wrapper is a Python OpenVINO GenAI binding and is not available in the native C++ session bridge");
    }
    throw std::runtime_error("unsupported OpenVINO incremental parser protocol: " + protocol);
}

static std::vector<std::shared_ptr<ov::genai::IncrementalParser>> incremental_parsers_for_protocols(const std::vector<std::string> &protocols) {
    std::vector<std::shared_ptr<ov::genai::IncrementalParser>> parsers;
    parsers.reserve(protocols.size());
    for (const auto &protocol : protocols) {
        parsers.push_back(incremental_parser_for_protocol(protocol));
    }
    return parsers;
}

static std::vector<std::shared_ptr<ov::genai::Parser>> parsers_for_protocols(const std::vector<std::string> &protocols) {
    std::vector<std::shared_ptr<ov::genai::Parser>> parsers;
    parsers.reserve(protocols.size());
    for (const auto &protocol : protocols) {
        parsers.push_back(parser_for_protocol(protocol));
    }
    return parsers;
}

static void apply_structured_output(ov::genai::GenerationConfig &gen,
                                    const char *protocol_c,
                                    const char *payload_c) {
    std::string protocol = (protocol_c && protocol_c[0]) ? std::string(protocol_c) : std::string();
    if (protocol.empty()) {
        return;
    }
    std::string payload = (payload_c && payload_c[0]) ? std::string(payload_c) : std::string();
    ov::genai::StructuredOutputConfig cfg;
    if (protocol == "openvino:json_schema") {
        cfg.json_schema = payload;
    } else if (protocol == "openvino:regex") {
        cfg.regex = payload;
    } else if (protocol == "openvino:ebnf") {
        cfg.grammar = payload;
    } else if (protocol == "openvino:const_string") {
        cfg.structural_tags_config = ov::genai::StructuredOutputConfig::StructuralTag{
            ov::genai::StructuredOutputConfig::ConstString(payload)};
    } else if (protocol == "openvino:any_text") {
        cfg.structural_tags_config = ov::genai::StructuredOutputConfig::StructuralTag{
            ov::genai::StructuredOutputConfig::AnyText{}};
    } else if (protocol == "openvino:structural_tag" ||
               protocol == "openvino:triggered_tags" ||
               protocol == "openvino:tags_with_separator" ||
               protocol == "openvino:concat" ||
               protocol == "openvino:union") {
        cfg.structural_tags_config = ov::genai::StructuredOutputConfig::StructuralTag{payload};
    } else {
        throw std::runtime_error("unsupported OpenVINO structured-output protocol: " + protocol);
    }
    cfg.validate();
    gen.structured_output_config = cfg;
}

static std::string parse_generated(const std::vector<std::shared_ptr<ov::genai::Parser>> &parsers,
                                   const std::string &generated) {
    if (parsers.empty()) {
        return std::string();
    }
    ov::genai::JsonContainer message;
    message["content"] = generated;
    for (auto &parser : parsers) {
        parser->parse(message);
    }
    return message.to_json_string();
}

static std::string json_string_field(const ov::genai::JsonContainer &message, const std::string &key) {
    if (!message.contains(key)) {
        return std::string();
    }
    auto value = message[key].as_string();
    return value.has_value() ? *value : std::string();
}

extern "C" {

cx_genai_session *cx_genai_session_new(const char *model_dir, const char *device,
                                       const cx_genai_session_config *config,
                                       char *err, size_t err_len) {
    try {
        auto *s = new cx_genai_session();
        std::string model_path(model_dir ? model_dir : "");
        std::string dev = (device && device[0]) ? std::string(device) : std::string("CPU");
        cx_genai_session_config cfg_copy{};
        if (config) {
            cfg_copy = *config;
        } else {
            cfg_copy.cache_size = 1;
            cfg_copy.dynamic_split_fuse = 1;
            cfg_copy.enable_prefix_caching = 1;
            cfg_copy.use_sparse_attention = 1;
            cfg_copy.num_last_dense_tokens_in_prefill = 10;
            cfg_copy.xattention_threshold = 0.9f;
            cfg_copy.xattention_block_size = 128;
            cfg_copy.xattention_stride = 16;
        }
        std::string kv_precision = (config && config->kv_cache_precision)
            ? std::string(config->kv_cache_precision)
            : std::string("f16");
        cfg_copy.kv_cache_precision = kv_precision.c_str();
        s->run([s, model_path, dev, cfg_copy, kv_precision] {
            auto cfg = scheduler_config_from(&cfg_copy);
            ov::AnyMap properties{{"KV_CACHE_PRECISION", kv_precision_from(&cfg_copy)}};
            s->pipe = std::make_unique<ov::genai::ContinuousBatchingPipeline>(
                std::filesystem::path(model_path),
                cfg,
                dev,
                properties);
        });
        return s;
    } catch (const std::exception &e) {
        write_buf(err, err_len, e.what());
        return nullptr;
    }
}

void cx_genai_session_free(cx_genai_session *s) {
    delete s;
}

int cx_genai_session_cancel(cx_genai_session *s) {
    if (!s) {
        return 1;
    }
    s->cancel_requested.store(true);
    return 0;
}

int cx_genai_apply_chat_template(cx_genai_session *s,
                                 const char **roles,
                                 const char **contents,
                                 const char **tool_calls,
                                 const char **tool_call_ids,
                                 size_t n,
                                 const char *tools_json,
                                 char *out,
                                 size_t out_len,
                                 char *err,
                                 size_t err_len) {
    if (!s) {
        write_buf(err, err_len, "OpenVINO GenAI session is nil");
        return 1;
    }
    try {
        std::string templated;
        std::string tools_str = (tools_json && tools_json[0]) ? std::string(tools_json) : std::string();
        s->run([&] {
            if (!s->pipe) {
                throw std::runtime_error("OpenVINO GenAI session is closed");
            }
            std::vector<ov::AnyMap> msgs;
            msgs.reserve(n);
            for (size_t i = 0; i < n; i++) {
                std::string role = (roles && roles[i]) ? std::string(roles[i]) : std::string();
                std::string content = (contents && contents[i]) ? std::string(contents[i]) : std::string();
                ov::AnyMap msg{{"role", role}, {"content", content}};
                if (tool_calls && tool_calls[i] && tool_calls[i][0] != '\0') {
                    msg["tool_calls"] = ov::genai::JsonContainer::from_json_string(std::string(tool_calls[i]));
                }
                if (tool_call_ids && tool_call_ids[i] && tool_call_ids[i][0] != '\0') {
                    msg["tool_call_id"] = std::string(tool_call_ids[i]);
                }
                msgs.push_back(msg);
            }
            ov::genai::ChatHistory history(msgs);
            std::optional<ov::genai::JsonContainer> tools;
            if (!tools_str.empty()) {
                tools = ov::genai::JsonContainer::from_json_string(tools_str);
            }
            templated = s->pipe->get_tokenizer().apply_chat_template(history, /*add_generation_prompt=*/true, std::string{}, tools);
        });
        if (templated.size() + 1 > out_len) {
            write_buf(err, err_len, "OpenVINO GenAI chat template output buffer too small");
            return 2;
        }
        write_buf(out, out_len, templated);
        return 0;
    } catch (const std::exception &e) {
        write_buf(err, err_len, e.what());
        return 1;
    }
}

int cx_genai_tokenize(cx_genai_session *s,
                      const char *prompt,
                      int add_special_tokens,
                      int64_t *tokens,
                      size_t tokens_len,
                      size_t *tokens_out,
                      char *err,
                      size_t err_len) {
    if (!s) {
        write_buf(err, err_len, "OpenVINO GenAI session is nil");
        return 1;
    }
    if (!prompt) {
        write_buf(err, err_len, "OpenVINO GenAI prompt is nil");
        return 1;
    }
    try {
        std::vector<int64_t> ids;
        std::string prompt_text(prompt);
        s->run([&] {
            if (!s->pipe) {
                throw std::runtime_error("OpenVINO GenAI session is closed");
            }
            auto encoded = s->pipe->get_tokenizer().encode(
                prompt_text,
                ov::genai::add_special_tokens(add_special_tokens != 0));
            auto input_ids = encoded.input_ids;
            const auto n = input_ids.get_size();
            const int64_t *src = input_ids.data<const int64_t>();
            ids.assign(src, src + n);
        });
        if (tokens_out) {
            *tokens_out = ids.size();
        }
        if (ids.size() > tokens_len) {
            write_buf(err, err_len, "OpenVINO GenAI token buffer too small");
            return 2;
        }
        if (!ids.empty() && !tokens) {
            write_buf(err, err_len, "OpenVINO GenAI token buffer is nil");
            return 1;
        }
        for (size_t i = 0; i < ids.size(); i++) {
            tokens[i] = ids[i];
        }
        return 0;
    } catch (const std::exception &e) {
        write_buf(err, err_len, e.what());
        return 1;
    }
}

int cx_genai_generate(cx_genai_session *s,
                      const char *prompt,
                      size_t max_new_tokens,
                      float temperature,
                      int use_temperature,
                      float top_p,
                      int use_top_p,
                      const char *structured_protocol,
                      const char *structured_payload,
                      const char *parser_protocols,
                      char *out,
                      size_t out_len,
                      char *parsed,
                      size_t parsed_len,
                      cx_genai_metrics *metrics,
                      char *err,
                      size_t err_len) {
    if (!s) {
        write_buf(err, err_len, "OpenVINO GenAI session is nil");
        return 1;
    }
    if (!prompt) {
        write_buf(err, err_len, "OpenVINO GenAI prompt is nil");
        return 1;
    }

    try {
        std::string generated;
        std::string parsed_message;
        ov::genai::PipelineMetrics latest_metrics;
        std::string prompt_text(prompt);
        std::vector<std::string> protocols = split_protocols(parser_protocols);
        bool canceled = false;

        s->run([&] {
            if (!s->pipe) {
                throw std::runtime_error("OpenVINO GenAI session is closed");
            }
            s->cancel_requested.store(false);

            auto gen = generation_config_from(s->pipe->get_config(), max_new_tokens, temperature, use_temperature, top_p, use_top_p);
            apply_structured_output(gen, structured_protocol, structured_payload);
            auto parsers = parsers_for_protocols(protocols);
            gen.parsers = parsers;
            gen.validate();

            ov::genai::StreamerVariant streamer = std::function<ov::genai::StreamingStatus(std::string)>(
                [s](std::string) {
                    if (s->cancel_requested.load()) {
                        return ov::genai::StreamingStatus::CANCEL;
                    }
                    return ov::genai::StreamingStatus::RUNNING;
                });
            auto results = s->pipe->generate(
                std::vector<std::string>{prompt_text},
                std::vector<ov::genai::GenerationConfig>{gen},
                streamer);
            canceled = s->cancel_requested.load();
            if (results.empty() || results[0].m_generation_ids.empty()) {
                if (canceled) {
                    return;
                }
                throw std::runtime_error("OpenVINO GenAI returned no generation");
            }
            generated = results[0].m_generation_ids[0];
            parsed_message = parse_generated(parsers, generated);
            latest_metrics = s->pipe->get_metrics();
        });

        if (canceled) {
            return 3;
        }
        if (generated.size() + 1 > out_len) {
            write_buf(err, err_len, "OpenVINO GenAI output buffer too small");
            return 2;
        }
        write_buf(out, out_len, generated);
        if (!parsed_message.empty()) {
            if (parsed_message.size() + 1 > parsed_len) {
                write_buf(err, err_len, "OpenVINO GenAI parsed output buffer too small");
                return 2;
            }
            write_buf(parsed, parsed_len, parsed_message);
        }
        copy_metrics(latest_metrics, metrics);
        return 0;
    } catch (const std::exception &e) {
        write_buf(err, err_len, e.what());
        return 1;
    }
}

cx_genai_stream *cx_genai_stream_new(void) {
    try {
        return new cx_genai_stream();
    } catch (...) {
        return nullptr;
    }
}

void cx_genai_stream_free(cx_genai_stream *stream) {
    delete stream;
}

void cx_genai_stream_abort(cx_genai_stream *stream, const char *message) {
    if (!stream) return;
    stream->finish(1, message ? std::string(message) : std::string("OpenVINO GenAI stream aborted"));
}

int cx_genai_stream_next(cx_genai_stream *stream,
                         char *out,
                         size_t out_len,
                         char *thinking,
                         size_t thinking_len,
                         char *err,
                         size_t err_len) {
    if (!stream) {
        write_buf(err, err_len, "OpenVINO GenAI stream is nil");
        return 2;
    }

    std::unique_lock<std::mutex> lock(stream->mu);
    stream->cv.wait(lock, [stream] {
        return !stream->chunks.empty() || stream->done;
    });

    if (!stream->chunks.empty()) {
        cx_genai_stream_chunk chunk = std::move(stream->chunks.front());
        stream->chunks.pop_front();
        lock.unlock();
        if (chunk.text.size() + 1 > out_len) {
            write_buf(err, err_len, "OpenVINO GenAI stream chunk buffer too small");
            return 2;
        }
        if (chunk.thinking.size() + 1 > thinking_len) {
            write_buf(err, err_len, "OpenVINO GenAI stream thinking buffer too small");
            return 2;
        }
        write_buf(out, out_len, chunk.text);
        write_buf(thinking, thinking_len, chunk.thinking);
        return 0;
    }

    int rc = stream->rc;
    std::string message = stream->error;
    lock.unlock();
    if (rc != 0) {
        write_buf(err, err_len, message);
        return rc;
    }
    return 1;
}

int cx_genai_generate_stream(cx_genai_session *s,
                             const char *prompt,
                             size_t max_new_tokens,
                             float temperature,
                             int use_temperature,
                             float top_p,
                             int use_top_p,
                             const char *parser_protocols,
                             cx_genai_stream *stream,
                             cx_genai_metrics *metrics,
                             char *err,
                             size_t err_len) {
    if (!s) {
        write_buf(err, err_len, "OpenVINO GenAI session is nil");
        if (stream) stream->finish(1, "OpenVINO GenAI session is nil");
        return 1;
    }
    if (!prompt) {
        write_buf(err, err_len, "OpenVINO GenAI prompt is nil");
        if (stream) stream->finish(1, "OpenVINO GenAI prompt is nil");
        return 1;
    }
    if (!stream) {
        write_buf(err, err_len, "OpenVINO GenAI stream is nil");
        return 1;
    }

    try {
        ov::genai::PipelineMetrics latest_metrics;
        std::string prompt_text(prompt);
        bool canceled = false;
        std::vector<std::string> protocols;
        if (parser_protocols && parser_protocols[0] != '\0') {
            std::stringstream ss(parser_protocols);
            std::string item;
            while (std::getline(ss, item, '\n')) {
                if (!item.empty()) {
                    protocols.push_back(item);
                }
            }
        }

        s->run([&] {
            if (!s->pipe) {
                throw std::runtime_error("OpenVINO GenAI session is closed");
            }
            s->cancel_requested.store(false);

            auto inc_parsers = incremental_parsers_for_protocols(protocols);

            auto gen = generation_config_from(s->pipe->get_config(), max_new_tokens, temperature, use_temperature, top_p, use_top_p);
            
            ov::genai::StreamerVariant streamer_variant = std::function<ov::genai::StreamingStatus(std::string)>(
                [s, stream, inc_parsers](std::string chunk) mutable {
                    ov::genai::JsonContainer delta_message;
                    for (auto &parser : inc_parsers) {
                        chunk = parser->parse(delta_message, chunk);
                    }
                    stream->push(chunk, json_string_field(delta_message, "reasoning_content"));
                    if (s->cancel_requested.load()) {
                        return ov::genai::StreamingStatus::CANCEL;
                    }
                    return ov::genai::StreamingStatus::RUNNING;
                });
            s->pipe->generate(
                std::vector<std::string>{prompt_text},
                std::vector<ov::genai::GenerationConfig>{gen},
                streamer_variant);
            canceled = s->cancel_requested.load();
            latest_metrics = s->pipe->get_metrics();
        });

        if (canceled) {
            stream->finish(3, "OpenVINO GenAI generation canceled");
            return 3;
        }
        copy_metrics(latest_metrics, metrics);
        stream->finish(0);
        return 0;
    } catch (const std::exception &e) {
        write_buf(err, err_len, e.what());
        stream->finish(1, e.what());
        return 1;
    }
}

}
