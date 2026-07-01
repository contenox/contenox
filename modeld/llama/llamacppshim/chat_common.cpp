//go:build llamacpp_direct

#include "chat.h"
#include "chat-peg-parser.h"
#include "llama.h"

#include <nlohmann/json.hpp>

#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <exception>
#include <string>
#include <stdexcept>

using json = nlohmann::ordered_json;

extern "C" struct cx_chat_apply_result {
    char *prompt;
    char *parser;
    char *generation_prompt;
    int format;
};

extern "C" struct cx_chat_syntax_result {
    char *parser;
    int format;
};

static char *cx_strdup_string(const std::string & value) {
    char *out = static_cast<char *>(std::malloc(value.size() + 1));
    if (out == nullptr) {
        return nullptr;
    }
    std::memcpy(out, value.data(), value.size());
    out[value.size()] = '\0';
    return out;
}

static common_reasoning_format cx_reasoning_format_or_none(const char *name) {
    if (name == nullptr || name[0] == '\0') {
        return COMMON_REASONING_FORMAT_NONE;
    }
    return common_reasoning_format_from_name(std::string(name));
}

extern "C" struct cx_chat_apply_result cx_common_chat_apply(
    const struct llama_model *model,
    const char *messages_json,
    const char *tools_json,
    int add_generation_prompt,
    const char *reasoning_format,
    int enable_thinking,
    char *errbuf,
    size_t errlen) {
    struct cx_chat_apply_result result = {};
    try {
        if (model == nullptr) {
            throw std::runtime_error("model is null");
        }

        auto tmpls = common_chat_templates_init(model, "");

        common_chat_templates_inputs inputs;
        inputs.use_jinja = true;
        inputs.add_generation_prompt = add_generation_prompt != 0;
        inputs.reasoning_format = cx_reasoning_format_or_none(reasoning_format);
        inputs.enable_thinking = enable_thinking != 0;
        inputs.messages = common_chat_msgs_parse_oaicompat(
            json::parse(messages_json != nullptr ? messages_json : "[]"));
        if (tools_json != nullptr && tools_json[0] != '\0') {
            inputs.tools = common_chat_tools_parse_oaicompat(json::parse(tools_json));
        }

        auto params = common_chat_templates_apply(tmpls.get(), inputs);
        char *out = cx_strdup_string(params.prompt);
        char *parser = cx_strdup_string(params.parser);
        char *generation_prompt = cx_strdup_string(params.generation_prompt);
        if (out == nullptr || parser == nullptr || generation_prompt == nullptr) {
            std::free(out);
            std::free(parser);
            std::free(generation_prompt);
            throw std::runtime_error("out of memory");
        }
        result.prompt = out;
        result.parser = parser;
        result.generation_prompt = generation_prompt;
        result.format = static_cast<int>(params.format);
        return result;
    } catch (const std::exception &e) {
        if (errbuf != nullptr && errlen > 0) {
            std::snprintf(errbuf, errlen, "%s", e.what());
        }
        return result;
    } catch (...) {
        if (errbuf != nullptr && errlen > 0) {
            std::snprintf(errbuf, errlen, "unknown common chat template error");
        }
        return result;
    }
}

extern "C" int cx_common_chat_parse(
    const char *input,
    int is_partial,
    int format,
    const char *parser_json,
    const char *generation_prompt,
    const char *reasoning_format,
    int parse_tool_calls,
    char **content_out,
    char **reasoning_out,
    char **tool_calls_out,
    char *errbuf,
    size_t errlen) {
    try {
        if (content_out == nullptr || reasoning_out == nullptr || tool_calls_out == nullptr) {
            throw std::runtime_error("output pointers are null");
        }
        *content_out = nullptr;
        *reasoning_out = nullptr;
        *tool_calls_out = nullptr;

        common_chat_parser_params params;
        params.format = static_cast<common_chat_format>(format);
        params.reasoning_format = cx_reasoning_format_or_none(reasoning_format);
        params.reasoning_in_content = false;
        params.parse_tool_calls = parse_tool_calls != 0;
        params.generation_prompt = generation_prompt != nullptr ? std::string(generation_prompt) : std::string();
        if (parser_json != nullptr && parser_json[0] != '\0') {
            params.parser.load(std::string(parser_json));
        }

        auto msg = common_chat_parse(
            std::string(input != nullptr ? input : ""),
            is_partial != 0,
            params);
        json tool_calls = json::array();
        for (const auto & tc : msg.tool_calls) {
            json item {
                {"type", "function"},
                {"function", {
                    {"name", tc.name},
                    {"arguments", tc.arguments},
                }},
            };
            if (!tc.id.empty()) {
                item["id"] = tc.id;
            }
            tool_calls.push_back(item);
        }
        *content_out = cx_strdup_string(msg.content);
        *reasoning_out = cx_strdup_string(msg.reasoning_content);
        *tool_calls_out = cx_strdup_string(tool_calls.dump());
        if (*content_out == nullptr || *reasoning_out == nullptr || *tool_calls_out == nullptr) {
            if (*content_out != nullptr) {
                std::free(*content_out);
                *content_out = nullptr;
            }
            if (*reasoning_out != nullptr) {
                std::free(*reasoning_out);
                *reasoning_out = nullptr;
            }
            if (*tool_calls_out != nullptr) {
                std::free(*tool_calls_out);
                *tool_calls_out = nullptr;
            }
            throw std::runtime_error("out of memory");
        }
        return 0;
    } catch (const std::exception &e) {
        if (errbuf != nullptr && errlen > 0) {
            std::snprintf(errbuf, errlen, "%s", e.what());
        }
        return -1;
    } catch (...) {
        if (errbuf != nullptr && errlen > 0) {
            std::snprintf(errbuf, errlen, "unknown common chat parse error");
        }
        return -1;
    }
}

extern "C" struct cx_chat_syntax_result cx_common_chat_syntax_openai_tools(
    const char *tools_json,
    char *errbuf,
    size_t errlen) {
    struct cx_chat_syntax_result result = {};
    try {
        auto tools = json::parse(tools_json != nullptr ? tools_json : "[]");
        auto parser = build_chat_peg_parser([&](common_chat_peg_builder & p) {
            return p.standard_json_tools(
                "{\"tool_calls\":",
                "}",
                tools,
                true,
                true,
                "function.name",
                "function.arguments",
                true,
                false,
                "id") + p.end();
        });
        result.parser = cx_strdup_string(parser.save());
        if (result.parser == nullptr) {
            throw std::runtime_error("out of memory");
        }
        result.format = static_cast<int>(COMMON_CHAT_FORMAT_PEG_NATIVE);
        return result;
    } catch (const std::exception &e) {
        if (errbuf != nullptr && errlen > 0) {
            std::snprintf(errbuf, errlen, "%s", e.what());
        }
        return result;
    } catch (...) {
        if (errbuf != nullptr && errlen > 0) {
            std::snprintf(errbuf, errlen, "unknown common chat syntax error");
        }
        return result;
    }
}
