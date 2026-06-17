//go:build llamanode

// Model-native chat-template rendering via minja — the header-only Jinja engine
// llama.cpp itself uses to apply a model's own GGUF chat template, including tool
// definitions. The legacy llama_chat_apply_template only matches a fixed list of
// built-in formats and does NOT execute the GGUF's Jinja (so it cannot render
// tools); this shim runs the real template the model author shipped.
#include <minja/chat-template.hpp>
#include <nlohmann/json.hpp>

#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <exception>
#include <string>

extern "C" char *cx_minja_apply(
    const char *source, const char *bos, const char *eos,
    const char *messages_json, const char *tools_json,
    int add_generation_prompt, char *errbuf, size_t errlen) {
    try {
        minja::chat_template tmpl(source ? source : "", bos ? bos : "", eos ? eos : "");

        minja::chat_template_inputs inputs;
        inputs.messages = nlohmann::ordered_json::parse(messages_json ? messages_json : "[]");
        if (tools_json && tools_json[0] != '\0') {
            inputs.tools = nlohmann::ordered_json::parse(tools_json);
        }
        inputs.add_generation_prompt = add_generation_prompt != 0;

        std::string out = tmpl.apply(inputs);

        char *res = static_cast<char *>(std::malloc(out.size() + 1));
        if (res == nullptr) {
            if (errbuf && errlen) std::snprintf(errbuf, errlen, "out of memory");
            return nullptr;
        }
        std::memcpy(res, out.data(), out.size());
        res[out.size()] = '\0';
        return res;
    } catch (const std::exception &e) {
        if (errbuf && errlen) std::snprintf(errbuf, errlen, "%s", e.what());
        return nullptr;
    } catch (...) {
        if (errbuf && errlen) std::snprintf(errbuf, errlen, "unknown minja error");
        return nullptr;
    }
}
