import { Badge } from "../Badge";
import { Table, TableCell, TableRow } from "../Table";
import type { CapturedStateUnit } from "./taskTypes";

export interface StateVisualizerLabels {
  taskId?: string;
  taskType?: string;
  inputType?: string;
  outputType?: string;
  transition?: string;
  duration?: string;
  error?: string;
}

interface StateVisualizerProps {
  state: CapturedStateUnit[];
  labels?: StateVisualizerLabels;
}

const DEFAULT_LABELS: Required<StateVisualizerLabels> = {
  taskId: "Task",
  taskType: "Type",
  inputType: "Input",
  outputType: "Output",
  transition: "Transition",
  duration: "Duration",
  error: "Error",
};

export const StateVisualizer = ({ state, labels }: StateVisualizerProps) => {
  if (!state || state.length === 0) {
    return null;
  }

  const l = { ...DEFAULT_LABELS, ...labels };

  // duration is a Go time.Duration serialized as nanoseconds; pick a readable unit.
  const formatDuration = (ns: number): string => {
    if (ns < 1000) return `${Math.round(ns)} ns`;
    const us = ns / 1000;
    if (us < 1000) return `${Math.round(us)} µs`;
    const ms = us / 1000;
    if (ms < 1000) return `${Math.round(ms)} ms`;
    return `${(ms / 1000).toFixed(2)} s`;
  };

  return (
    <Table
      columns={[
        l.taskId,
        l.taskType,
        l.inputType,
        l.outputType,
        l.transition,
        l.duration,
        l.error,
      ]}>
      {state.map((unit, index) => (
        <TableRow key={index} className={unit.error ? "bg-error/10" : ""}>
          <TableCell>{unit.taskID}</TableCell>
          <TableCell>{unit.taskHandler}</TableCell>
          <TableCell>{unit.inputType}</TableCell>
          <TableCell>{unit.outputType}</TableCell>
          <TableCell className="max-w-xs truncate">{unit.transition || "-"}</TableCell>
          <TableCell>{formatDuration(unit.duration)}</TableCell>
          <TableCell>
            {unit.error?.error ? (
              <Badge variant="error" size="sm">
                {unit.error.error}
              </Badge>
            ) : (
              "-"
            )}
          </TableCell>
        </TableRow>
      ))}
    </Table>
  );
};
