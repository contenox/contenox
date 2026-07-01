#!/usr/bin/env python3
import argparse
import json
import time
import traceback
from pathlib import Path

import openvino_genai as ovg


def main():
    parser = argparse.ArgumentParser(description="Raw OpenVINO GenAI ContinuousBatchingPipeline benchmark.")
    parser.add_argument("--model-dir", required=True)
    parser.add_argument("--prompt", required=True)
    parser.add_argument("--out-dir", required=True)
    parser.add_argument("--model-name", default="")
    parser.add_argument("--devices", nargs="+", default=["GPU"])
    parser.add_argument("--max-new-tokens", type=int, default=64)
    parser.add_argument("--cache-size", type=int, default=1)
    parser.add_argument("--sparse", action="store_true")
    args = parser.parse_args()

    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    result_path = out_dir / "results.jsonl"
    if result_path.exists():
        result_path.unlink()

    prompt = Path(args.prompt).read_text(encoding="utf-8")
    for device in args.devices:
        row = {
            "device": device,
            "model": args.model_name or Path(args.model_dir).name,
            "pipeline": "ContinuousBatchingPipeline",
            "sparse_attention": bool(args.sparse),
            "cache_size": args.cache_size,
            "prompt_chars": len(prompt),
            "max_new_tokens": args.max_new_tokens,
        }
        try:
            scheduler = ovg.SchedulerConfig()
            scheduler.cache_size = args.cache_size
            scheduler.use_sparse_attention = bool(args.sparse)

            t0 = time.perf_counter()
            pipe = ovg.ContinuousBatchingPipeline(args.model_dir, scheduler, device)
            row["load_ms"] = round((time.perf_counter() - t0) * 1000, 3)

            generation = ovg.GenerationConfig()
            generation.max_new_tokens = args.max_new_tokens
            generation.do_sample = False

            t1 = time.perf_counter()
            result = pipe.generate(prompt, generation)
            row["generate_ms"] = round((time.perf_counter() - t1) * 1000, 3)
            row["wall_ms"] = round(row["load_ms"] + row["generate_ms"], 3)

            first = result[0] if isinstance(result, list) and result else result
            text = generated_text(first)
            row["output_chars"] = len(text)
            (out_dir / f"{device.lower()}.out.txt").write_text(text, encoding="utf-8")

            add_perf_metrics(row, first)

            try:
                tokenized = pipe.get_tokenizer().encode(text, add_special_tokens=False)
                completion_tokens = int(tokenized.input_ids.get_size())
                row["completion_tokens_est"] = completion_tokens
                if completion_tokens > 0 and row["generate_ms"] > 0:
                    row["tokens_per_second_generate_est"] = round(completion_tokens / (row["generate_ms"] / 1000), 3)
                    row["tokens_per_second_wall_est"] = round(completion_tokens / (row["wall_ms"] / 1000), 3)
            except Exception as exc:
                row["completion_tokens_est_error"] = repr(exc)

            row["success"] = True
        except Exception as exc:
            row["success"] = False
            row["error"] = repr(exc)
            row["traceback"] = traceback.format_exc()

        print(json.dumps(row, ensure_ascii=False), flush=True)
        with result_path.open("a", encoding="utf-8") as f:
            f.write(json.dumps(row, ensure_ascii=False) + "\n")


def generated_text(result):
    if result is None:
        return ""
    text = getattr(result, "text", "")
    if text:
        return text
    ids = getattr(result, "m_generation_ids", None)
    if ids:
        return str(ids[0])
    get_ids = getattr(result, "get_generation_ids", None)
    if callable(get_ids):
        ids = get_ids()
        if ids:
            return str(ids[0])
    return str(result)


def add_perf_metrics(row, result):
    metrics = getattr(result, "perf_metrics", None)
    if metrics is None:
        return
    int_metrics = {
        "ov_num_input_tokens": "get_num_input_tokens",
        "ov_num_generated_tokens": "get_num_generated_tokens",
    }
    for row_key, method_name in int_metrics.items():
        value = call_metric(metrics, method_name)
        if value is not None:
            row[row_key] = int(value)

    load_time = call_metric(metrics, "get_load_time")
    if load_time is not None:
        row["ov_load_time_ms"] = round(float(load_time), 3)

    pair_metrics = {
        "ov_generate_duration_ms": "get_generate_duration",
        "ov_inference_duration_ms": "get_inference_duration",
        "ov_tokenization_duration_ms": "get_tokenization_duration",
        "ov_detokenization_duration_ms": "get_detokenization_duration",
        "ov_ttft_ms": "get_ttft",
        "ov_tpot_ms": "get_tpot",
        "ov_throughput_tps": "get_throughput",
    }
    for row_key, method_name in pair_metrics.items():
        pair = call_metric(metrics, method_name)
        mean = getattr(pair, "mean", None)
        if mean is not None:
            row[row_key] = round(float(mean), 3)
        std = getattr(pair, "std", None)
        if std is not None:
            row[f"{row_key}_std"] = round(float(std), 3)


def call_metric(metrics, method_name):
    method = getattr(metrics, method_name, None)
    if not callable(method):
        return None
    try:
        return method()
    except Exception:
        return None


if __name__ == "__main__":
    main()
