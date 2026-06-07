"use strict";
var __create = Object.create;
var __defProp = Object.defineProperty;
var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
var __getOwnPropNames = Object.getOwnPropertyNames;
var __getProtoOf = Object.getPrototypeOf;
var __hasOwnProp = Object.prototype.hasOwnProperty;
var __export = (target, all) => {
  for (var name in all)
    __defProp(target, name, { get: all[name], enumerable: true });
};
var __copyProps = (to, from, except, desc) => {
  if (from && typeof from === "object" || typeof from === "function") {
    for (let key of __getOwnPropNames(from))
      if (!__hasOwnProp.call(to, key) && key !== except)
        __defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
  }
  return to;
};
var __toESM = (mod, isNodeMode, target) => (target = mod != null ? __create(__getProtoOf(mod)) : {}, __copyProps(
  // If the importer is in node compatibility mode or this is not an ESM
  // file that has been converted to a CommonJS file using a Babel-
  // compatible transform (i.e. "__esModule" has not been set), then set
  // "default" to the CommonJS "module.exports" for node compatibility.
  isNodeMode || !mod || !mod.__esModule ? __defProp(target, "default", { value: mod, enumerable: true }) : target,
  mod
));
var __toCommonJS = (mod) => __copyProps(__defProp({}, "__esModule", { value: true }), mod);

// src/index.ts
var index_exports = {};
__export(index_exports, {
  Accordion: () => Accordion,
  AddNodeButton: () => AddNodeButton,
  ApprovalCard: () => ApprovalCard,
  Badge: () => Badge,
  Blockquote: () => Blockquote,
  Button: () => Button,
  ButtonGroup: () => ButtonGroup,
  Card: () => Card,
  ChatComposer: () => ChatComposer,
  ChatDateSeparator: () => ChatDateSeparator,
  ChatMessage: () => ChatMessage,
  ChatProcessingBar: () => ChatProcessingBar,
  ChatScrollToLatest: () => ChatScrollToLatest,
  ChatStreamThinkingBox: () => ChatStreamThinkingBox,
  ChatStreamingCaret: () => ChatStreamingCaret,
  ChatThread: () => ChatThread,
  ChatThreadSkeleton: () => ChatThreadSkeleton,
  ChatTranscriptStreamingPlaceholder: () => ChatTranscriptStreamingPlaceholder,
  ChatTypingIndicator: () => ChatTypingIndicator,
  Checkbox: () => Checkbox,
  CodeBlock: () => CodeBlock,
  Collapsible: () => Collapsible,
  CollapsibleContent: () => CollapsibleContent,
  CollapsibleTrigger: () => CollapsibleTrigger,
  CommandPanel: () => CommandPanel,
  Container: () => Container,
  DEFAULT_COMPOSER_SOFT_MAX: () => DEFAULT_COMPOSER_SOFT_MAX,
  DetailsPanel: () => DetailsPanel,
  Dialog: () => Dialog,
  DiffView: () => DiffView,
  DragDropContextProvider: () => DragDropContextProvider,
  Draggable: () => Draggable,
  Dropdown: () => Dropdown,
  Droppable: () => Droppable,
  EmptyState: () => EmptyState,
  ErrorBoundary: () => ErrorBoundary,
  ErrorState: () => ErrorState,
  ExecutionTimeline: () => ExecutionTimeline,
  FileTree: () => FileTree,
  Fill: () => Fill,
  Form: () => Form,
  FormField: () => FormField,
  GridLayout: () => GridLayout,
  H1: () => H1,
  H2: () => H2,
  H3: () => H3,
  Inbox: () => Inbox,
  InlineAttachmentRenderer: () => InlineAttachmentRenderer,
  InlineAttachments: () => InlineAttachments,
  InlineNotice: () => InlineNotice,
  Input: () => Input,
  InsetPanel: () => InsetPanel,
  InsetPanelBody: () => InsetPanelBody,
  InsetPanelHeader: () => InsetPanelHeader,
  JsonEditor: () => JsonEditor_default,
  KeyValue: () => KeyValue,
  Label: () => Label,
  LayoutControls: () => LayoutControls,
  LoadingState: () => LoadingState,
  MonoLogList: () => MonoLogList,
  MonoLogListItem: () => MonoLogListItem,
  NavItem: () => NavItem,
  NumberInput: () => NumberInput,
  P: () => P,
  Page: () => Page,
  Pagination: () => Pagination,
  Panel: () => Panel,
  PasswordInput: () => PasswordInput,
  ProgressBar: () => ProgressBar,
  ResizablePanel: () => ResizablePanel,
  ResizablePanelGroup: () => ResizablePanelGroup,
  ResizablePanelHandle: () => ResizablePanelHandle,
  ResourceCard: () => ResourceCard,
  SearchBar: () => SearchBar,
  Section: () => Section,
  Select: () => Select,
  SelectOption: () => SelectOption,
  SidePanelBody: () => SidePanelBody,
  SidePanelColumn: () => SidePanelColumn,
  SidePanelHeader: () => SidePanelHeader,
  SidePanelRailButton: () => SidePanelRailButton,
  SidebarToggle: () => SidebarToggle,
  Skeleton: () => Skeleton,
  Small: () => Small,
  Span: () => Span,
  Spinner: () => Spinner,
  StateVisualizer: () => StateVisualizer,
  TabPanel: () => TabPanel,
  TabPanels: () => TabPanels,
  TabTrigger: () => TabTrigger,
  TabbedForm: () => TabbedForm,
  TabbedPage: () => TabbedPage,
  Table: () => Table,
  TableCell: () => TableCell,
  TableRow: () => TableRow,
  Tabs: () => Tabs,
  TaskEventFeed: () => TaskEventFeed,
  TerminalOutput: () => TerminalOutput,
  Textarea: () => Textarea,
  Toast: () => Toast,
  ToolCallCard: () => ToolCallCard,
  Toolbar: () => Toolbar,
  ToolbarActions: () => ToolbarActions,
  ToolbarItem: () => ToolbarItem,
  ToolbarSection: () => ToolbarSection,
  Tooltip: () => Tooltip,
  TranscriptEmbedCard: () => TranscriptEmbedCard,
  UserMenu: () => UserMenu,
  Wizard: () => Wizard,
  WizardStep: () => WizardStep,
  WorkflowEdge: () => WorkflowEdge,
  WorkflowNode: () => WorkflowNode,
  WorkflowVisualizer: () => WorkflowVisualizer,
  calculateLayout: () => calculateLayout,
  chatTranscriptMarkdownComponents: () => chatTranscriptMarkdownComponents,
  cn: () => cn,
  copyTextToClipboard: () => copyTextToClipboard,
  getConnectorPath: () => getConnectorPath,
  isComposerCharCountWarning: () => isComposerCharCountWarning,
  isOverComposerSoftMax: () => isOverComposerSoftMax,
  useChatScroll: () => useChatScroll,
  useCollapsibleContext: () => useCollapsibleContext,
  useDragDropContext: () => useDragDropContext
});
module.exports = __toCommonJS(index_exports);

// src/utils.ts
var import_clsx = require("clsx");
var import_tailwind_merge = require("tailwind-merge");
function cn(...inputs) {
  return (0, import_tailwind_merge.twMerge)((0, import_clsx.clsx)(inputs));
}

// src/components/DragDropContext.tsx
var import_react = require("react");
var import_jsx_runtime = require("react/jsx-runtime");
var DragDropContext = (0, import_react.createContext)(
  void 0
);
function dragReducer(state, action) {
  switch (action.type) {
    case "START_DRAG":
      return {
        draggedId: action.payload.draggableId,
        sourceDroppableId: action.payload.sourceDroppableId,
        destinationDroppableId: action.payload.sourceDroppableId
      };
    case "UPDATE_DRAG":
      return {
        ...state,
        destinationDroppableId: action.payload.destinationDroppableId
      };
    case "END_DRAG":
      return {
        draggedId: null,
        sourceDroppableId: null,
        destinationDroppableId: null
      };
    default:
      return state;
  }
}
function DragDropContextProvider({
  children,
  onDragEnd
}) {
  const [dragState, dispatch] = (0, import_react.useReducer)(dragReducer, {
    draggedId: null,
    sourceDroppableId: null,
    destinationDroppableId: null
  });
  const startDrag = (0, import_react.useCallback)(
    (draggableId, sourceDroppableId) => {
      dispatch({
        type: "START_DRAG",
        payload: { draggableId, sourceDroppableId }
      });
    },
    []
  );
  const updateDrag = (0, import_react.useCallback)((destinationDroppableId) => {
    dispatch({ type: "UPDATE_DRAG", payload: { destinationDroppableId } });
  }, []);
  const endDrag = (0, import_react.useCallback)(() => {
    if (dragState.draggedId && dragState.sourceDroppableId && dragState.destinationDroppableId) {
      onDragEnd({
        draggableId: dragState.draggedId,
        sourceDroppableId: dragState.sourceDroppableId,
        destinationDroppableId: dragState.destinationDroppableId
      });
    }
    dispatch({ type: "END_DRAG" });
  }, [dragState, onDragEnd]);
  const isDragging = (0, import_react.useCallback)(
    (draggableId) => {
      return dragState.draggedId === draggableId;
    },
    [dragState.draggedId]
  );
  const value = {
    dragState,
    startDrag,
    updateDrag,
    endDrag,
    isDragging
  };
  return /* @__PURE__ */ (0, import_jsx_runtime.jsx)(DragDropContext.Provider, { value, children });
}
function useDragDropContext() {
  const context = (0, import_react.useContext)(DragDropContext);
  if (context === void 0) {
    throw new Error(
      "useDragDropContext must be used within a DragDropContextProvider"
    );
  }
  return context;
}

// src/components/MonoLogList.tsx
var import_react2 = require("react");
var import_jsx_runtime2 = require("react/jsx-runtime");
var MonoLogList = (0, import_react2.forwardRef)(function MonoLogList2({ className, maxHeightClassName = "max-h-48", ...props }, ref) {
  return /* @__PURE__ */ (0, import_jsx_runtime2.jsx)(
    "ul",
    {
      ref,
      className: cn(
        "space-y-1 overflow-y-auto border border-dashed border-surface-300 px-2 py-1.5 font-mono text-[10px] dark:border-dark-surface-600",
        maxHeightClassName,
        className
      ),
      ...props
    }
  );
});
var MonoLogListItem = (0, import_react2.forwardRef)(
  function MonoLogListItem2({ className, ...props }, ref) {
    return /* @__PURE__ */ (0, import_jsx_runtime2.jsx)(
      "li",
      {
        ref,
        className: cn(
          "text-text dark:text-dark-text border-surface-200 border-b border-dotted pb-0.5 last:border-0 dark:border-dark-surface-600",
          className
        ),
        ...props
      }
    );
  }
);

// src/components/Input.tsx
var import_react3 = require("react");
var import_jsx_runtime3 = require("react/jsx-runtime");
var Input = (0, import_react3.forwardRef)(
  ({ className, startIcon, endIcon, error = false, ...props }, ref) => {
    return /* @__PURE__ */ (0, import_jsx_runtime3.jsxs)("div", { className: "relative", children: [
      startIcon && /* @__PURE__ */ (0, import_jsx_runtime3.jsx)("div", { className: "absolute top-1/2 left-3 -translate-y-1/2 text-secondary-400 dark:text-dark-secondary-400", children: startIcon }),
      /* @__PURE__ */ (0, import_jsx_runtime3.jsx)(
        "input",
        {
          ref,
          className: cn(
            // Base styles
            "bg-surface-50 text-text placeholder:text-text-muted rounded-lg border h-9 px-3 py-1 text-sm transition-colors",
            "focus:ring-primary-500 focus:ring-2 focus:ring-offset-2 focus:outline-none",
            "focus:ring-offset-surface-50 dark:focus:ring-offset-dark-surface-100",
            // Dark mode styles
            "dark:bg-dark-surface-50 dark:text-dark-text dark:placeholder:text-dark-text-muted dark:focus:ring-dark-primary-500 dark:focus:ring-offset-dark-surface-50",
            // Icon padding
            startIcon && "pl-10",
            endIcon && "pr-10",
            // Error and default border states
            error ? "border-error-300 focus:border-error-500 focus:ring-error-500 dark:border-dark-error-400 dark:focus:border-dark-error-500 dark:focus:ring-dark-error-500" : "border-surface-300 dark:border-dark-surface-600 focus:border-primary-500",
            "disabled:bg-surface-100 dark:disabled:bg-dark-surface-200 disabled:text-text-muted disabled:cursor-not-allowed",
            "placeholder:text-secondary-400 dark:placeholder:dark-secondary-400",
            className
          ),
          ...props
        }
      ),
      endIcon && /* @__PURE__ */ (0, import_jsx_runtime3.jsx)("div", { className: "absolute top-1/2 right-3 -translate-y-1/2 text-secondary-400 dark:text-dark-secondary-400", children: endIcon })
    ] });
  }
);
Input.displayName = "Input";
var PasswordInput = (0, import_react3.forwardRef)(
  ({ endIcon, ...props }, ref) => {
    const [showPassword, setShowPassword] = (0, import_react3.useState)(false);
    const toggleShowPassword = (e) => {
      e.preventDefault();
      setShowPassword((prev) => !prev);
    };
    const toggleIcon = /* @__PURE__ */ (0, import_jsx_runtime3.jsx)("button", { type: "button", onClick: toggleShowPassword, children: showPassword ? "Hide" : "Show" });
    return /* @__PURE__ */ (0, import_jsx_runtime3.jsx)(
      Input,
      {
        ...props,
        ref,
        type: showPassword ? "text" : "password",
        endIcon: toggleIcon
      }
    );
  }
);
PasswordInput.displayName = "PasswordInput";

// src/components/NumberInput.tsx
var import_jsx_runtime4 = require("react/jsx-runtime");
function NumberInput({
  value,
  onChange,
  className,
  ...props
}) {
  const handleChange = (e) => {
    const numValue = parseFloat(e.target.value);
    if (!isNaN(numValue)) {
      onChange(numValue);
    } else if (e.target.value === "") {
      onChange(0);
    }
  };
  return /* @__PURE__ */ (0, import_jsx_runtime4.jsx)(
    Input,
    {
      type: "number",
      value,
      onChange: handleChange,
      className,
      ...props
    }
  );
}

// src/components/Draggable.tsx
var import_jsx_runtime5 = require("react/jsx-runtime");
function Draggable({
  draggableId,
  children,
  className,
  isDragDisabled = false,
  index
}) {
  const { startDrag, endDrag, isDragging } = useDragDropContext();
  const handleDragStart = (e) => {
    if (isDragDisabled) return;
    e.dataTransfer.setData("text/plain", draggableId);
    e.dataTransfer.effectAllowed = "move";
    startDrag(draggableId, "default");
    if (e.dataTransfer && e.currentTarget instanceof HTMLElement) {
      e.dataTransfer.setDragImage(e.currentTarget, 20, 20);
    }
  };
  const handleDragEnd = (e) => {
    e.preventDefault();
    endDrag();
  };
  const dragging = isDragging(draggableId);
  return /* @__PURE__ */ (0, import_jsx_runtime5.jsx)(
    "div",
    {
      draggable: !isDragDisabled,
      onDragStart: handleDragStart,
      onDragEnd: handleDragEnd,
      className: cn(
        "cursor-grab active:cursor-grabbing transition-all duration-200 ease-in-out",
        dragging && "opacity-50 scale-95 shadow-lg",
        !isDragDisabled && "hover:shadow-md hover:bg-surface-50 dark:hover:bg-dark-surface-50",
        className
      ),
      "data-draggable-id": draggableId,
      "data-index": index,
      children
    }
  );
}

// src/components/Collapsible.tsx
var import_react5 = __toESM(require("react"));

// src/components/Button.tsx
var import_react4 = require("react");

// src/components/Spinner.tsx
var import_jsx_runtime6 = require("react/jsx-runtime");
function Spinner({ className, size = "md" }) {
  return /* @__PURE__ */ (0, import_jsx_runtime6.jsxs)(
    "svg",
    {
      className: cn(
        "text-primary-500 dark:text-dark-primary-500 animate-spin",
        {
          "h-5 w-5": size === "sm",
          "h-6 w-6": size === "md",
          "h-8 w-8": size === "lg"
        },
        className
      ),
      xmlns: "http://www.w3.org/2000/svg",
      fill: "none",
      viewBox: "0 0 24 24",
      children: [
        /* @__PURE__ */ (0, import_jsx_runtime6.jsx)(
          "circle",
          {
            className: "opacity-25",
            cx: "12",
            cy: "12",
            r: "10",
            stroke: "currentColor",
            strokeWidth: "4"
          }
        ),
        /* @__PURE__ */ (0, import_jsx_runtime6.jsx)(
          "path",
          {
            className: "opacity-75",
            fill: "currentColor",
            d: "M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
          }
        )
      ]
    }
  );
}

// src/components/Typography.tsx
var import_jsx_runtime7 = require("react/jsx-runtime");
function H1({ children, className, variant }) {
  return /* @__PURE__ */ (0, import_jsx_runtime7.jsx)(
    "h1",
    {
      className: cn(
        "text-text dark:text-dark-text",
        variant === "hero" ? "text-5xl md:text-6xl font-bold leading-tight" : variant === "sectionTitle" ? "text-4xl font-display font-bold" : variant === "page" ? "text-2xl font-semibold" : "text-2xl font-bold",
        className
      ),
      children
    }
  );
}
function H2({ children, className, variant }) {
  return /* @__PURE__ */ (0, import_jsx_runtime7.jsx)(
    "h2",
    {
      className: cn(
        "font-semibold text-text dark:text-dark-text",
        variant === "footerTitle" ? "text-lg" : variant === "sectionTitle" ? "text-3xl font-display font-bold" : "text-xl",
        className
      ),
      children
    }
  );
}
function H3({ children, className, variant }) {
  return /* @__PURE__ */ (0, import_jsx_runtime7.jsx)(
    "h3",
    {
      className: cn(
        "text-text dark:text-dark-text",
        variant === "subsectionTitle" ? "text-xl font-semibold" : "text-lg font-medium",
        className
      ),
      children
    }
  );
}
function P({ children, className, variant }) {
  return /* @__PURE__ */ (0, import_jsx_runtime7.jsx)(
    "p",
    {
      className: cn(
        "text-text dark:text-dark-text",
        variant === "lead" ? "text-xl leading-relaxed" : variant === "cardSubtitle" ? "text-sm text-text-muted dark:text-dark-text-muted" : variant === "footerText" ? "text-sm text-text-muted dark:text-dark-text-muted" : variant === "body" ? "text-base" : variant === "caption" ? "text-xs text-text-muted uppercase tracking-wide" : "text-base",
        variant === "status" ? "text-xs uppercase tracking-wider font-medium" : className
      ),
      children
    }
  );
}
function Small({ children, className }) {
  return /* @__PURE__ */ (0, import_jsx_runtime7.jsx)("p", { className: cn("text-sm text-text dark:text-dark-text-muted", className), children });
}
function Span({ children, className, variant, ...props }) {
  return /* @__PURE__ */ (0, import_jsx_runtime7.jsx)(
    "span",
    {
      className: cn(
        "text-[var(--color-text)] dark:text-[var(--color-dark-text)]",
        {
          "text-xs uppercase tracking-wider font-medium": variant === "status",
          "text-sm text-text-muted dark:text-dark-text-muted": variant === "muted"
        },
        className
      ),
      ...props,
      children
    }
  );
}
function Blockquote({ children, className }) {
  return /* @__PURE__ */ (0, import_jsx_runtime7.jsx)(
    "blockquote",
    {
      className: cn(
        "border-l-4 pl-4 italic text-text dark:text-dark-text",
        className
      ),
      children
    }
  );
}

// src/components/Button.tsx
var import_jsx_runtime8 = require("react/jsx-runtime");
var Button = (0, import_react4.forwardRef)(
  ({
    className,
    variant = "primary",
    size = "md",
    palette = "primary",
    isLoading = false,
    textAlign = "center",
    ...props
  }, ref) => {
    const paletteStyles = {
      primary: cn(
        "text-white dark:text-dark-surface-50",
        variant !== "text" && "bg-primary dark:bg-dark-primary",
        "hover:bg-primary-600 dark:hover:bg-dark-primary-600",
        "focus:ring-primary-300 dark:focus:ring-dark-primary-300"
      ),
      secondary: cn(
        "text-on-secondary dark:text-dark-surface",
        variant !== "text" && "bg-secondary dark:bg-dark-secondary",
        "hover:bg-secondary-600 dark:hover:bg-dark-secondary-600",
        "focus:ring-secondary-300 dark:focus:ring-dark-secondary-300"
      ),
      accent: cn(
        "text-surface-inverted dark:text-dark-text",
        variant !== "text" && "bg-accent dark:bg-dark-accent",
        "hover:bg-accent-600 dark:hover:bg-dark-accent-600",
        "focus:ring-accent-300 dark:focus:ring-dark-accent-300"
      ),
      neutral: cn(
        "text-text dark:text-dark-text-muted",
        "hover:bg-surface-100 dark:hover:bg-dark-surface-100",
        "focus:ring-surface-300 dark:focus:ring-dark-surface-300"
      ),
      light: cn(
        "text-primary dark:text-dark-primary",
        "hover:bg-surface-50 dark:hover:bg-dark-surface-50",
        "focus:ring-primary-100 dark:focus:ring-dark-primary-100"
      )
    };
    const ghostStyles = cn(
      "bg-transparent",
      "hover:bg-surface-100 dark:hover:bg-dark-surface-100",
      "text-current"
    );
    const dangerStyles = cn(
      "bg-transparent",
      "text-error dark:text-dark-error",
      "hover:bg-error/10 dark:hover:bg-dark-error/10",
      "focus:ring-error-300 dark:focus:ring-dark-error-300"
    );
    const successStyles = cn(
      "bg-transparent",
      "text-success dark:text-dark-success",
      "hover:bg-success/10 dark:hover:bg-dark-success/10",
      "focus:ring-success-300 dark:focus:ring-dark-success-300"
    );
    return /* @__PURE__ */ (0, import_jsx_runtime8.jsx)(
      "button",
      {
        ref,
        className: cn(
          "inline-flex flex-row items-center justify-center",
          "ease-fluid rounded-lg transition-all focus:ring-2 focus:ring-offset-2 focus:outline-none",
          "disabled:cursor-not-allowed disabled:opacity-50",
          {
            "h-7 px-2 text-xs": size === "xs",
            "h-8 px-3 text-xs": size === "sm",
            "h-9 px-4 py-2 text-sm": size === "md",
            "h-10 px-8 text-sm": size === "lg",
            "px-10 py-4 text-xl": size === "xl",
            "px-12 py-5 text-2xl": size === "2xl",
            "p-2.5": size === "icon"
          },
          variant === "outline" && cn(
            "border-2 bg-transparent",
            palette === "primary" && "border-primary text-primary dark:border-dark-primary dark:text-dark-primary",
            palette === "secondary" && "border-secondary text-secondary dark:border-dark-secondary dark:text-dark-secondary",
            palette === "accent" && "border-accent text-accent dark:border-dark-accent dark:text-dark-accent",
            palette === "neutral" && "border-surface-200 text-text dark:border-dark-surface-200 dark:text-dark-text"
          ),
          variant === "text" && "bg-transparent hover:bg-opacity-10",
          variant === "ghost" ? ghostStyles : variant === "danger" ? dangerStyles : variant === "success" ? successStyles : variant !== "outline" && variant !== "text" && paletteStyles[palette],
          className
        ),
        disabled: isLoading || props.disabled,
        ...props,
        children: isLoading ? /* @__PURE__ */ (0, import_jsx_runtime8.jsxs)(Span, { className: "flex items-center gap-2", children: [
          /* @__PURE__ */ (0, import_jsx_runtime8.jsx)(
            Spinner,
            {
              size: size === "icon" || size === "xs" ? "sm" : size === "xl" || size === "2xl" ? "lg" : size,
              className: "mr-1"
            }
          ),
          /* @__PURE__ */ (0, import_jsx_runtime8.jsx)(Span, { className: cn(textAlign === "bottom" && "self-end"), children: props.children })
        ] }) : props.children
      }
    );
  }
);
Button.displayName = "Button";

// src/components/Collapsible.tsx
var import_jsx_runtime9 = require("react/jsx-runtime");
var CollapsibleContext = (0, import_react5.createContext)(
  void 0
);
function useCollapsibleContext() {
  const ctx = (0, import_react5.useContext)(CollapsibleContext);
  if (!ctx) {
    throw new Error("useCollapsibleContext must be used within Collapsible");
  }
  return ctx;
}
var Collapsible = ({
  open: controlledOpen,
  onOpenChange,
  defaultOpen,
  defaultExpanded,
  title,
  children,
  className
}) => {
  const [internalOpen, setInternalOpen] = (0, import_react5.useState)(() => {
    if (controlledOpen !== void 0) return controlledOpen;
    if (defaultOpen !== void 0) return defaultOpen;
    if (defaultExpanded !== void 0) return defaultExpanded;
    return false;
  });
  const isControlled = controlledOpen !== void 0;
  const open = isControlled ? controlledOpen : internalOpen;
  const handleOpenChange = (newOpen) => {
    if (!isControlled) {
      setInternalOpen(newOpen);
    }
    onOpenChange?.(newOpen);
  };
  return /* @__PURE__ */ (0, import_jsx_runtime9.jsx)(
    CollapsibleContext.Provider,
    {
      value: { open, onOpenChange: handleOpenChange },
      children: /* @__PURE__ */ (0, import_jsx_runtime9.jsx)("div", { className: cn("w-full", className), children: title ? /* @__PURE__ */ (0, import_jsx_runtime9.jsxs)(import_jsx_runtime9.Fragment, { children: [
        " ",
        /* @__PURE__ */ (0, import_jsx_runtime9.jsxs)(CollapsibleTrigger, { className: "flex w-full items-center justify-between rounded-md bg-surface-50 dark:bg-dark-surface-50 border border-surface-300 dark:border-dark-surface-300 px-3 py-2 text-left text-text dark:text-dark-text hover:bg-surface-100 dark:hover:bg-dark-surface-100", children: [
          " ",
          /* @__PURE__ */ (0, import_jsx_runtime9.jsx)("span", { children: title }),
          /* @__PURE__ */ (0, import_jsx_runtime9.jsx)(
            "span",
            {
              "aria-hidden": true,
              className: "text-text-muted dark:text-dark-text-muted",
              children: open ? "\u2212" : "+"
            }
          )
        ] }),
        /* @__PURE__ */ (0, import_jsx_runtime9.jsx)(CollapsibleContent, { children })
      ] }) : children })
    }
  );
};
var CollapsibleTrigger = ({
  asChild = false,
  children,
  className,
  ...props
}) => {
  const context = (0, import_react5.useContext)(CollapsibleContext);
  if (!context) {
    throw new Error("CollapsibleTrigger must be used within a Collapsible");
  }
  const { open, onOpenChange } = context;
  const handleClick = () => {
    onOpenChange(!open);
  };
  if (asChild && import_react5.default.isValidElement(children)) {
    return import_react5.default.cloneElement(children, {
      onClick: handleClick,
      "aria-expanded": open,
      "data-state": open ? "open" : "closed",
      ...props
    });
  }
  return /* @__PURE__ */ (0, import_jsx_runtime9.jsx)(
    Button,
    {
      type: "button",
      onClick: handleClick,
      "aria-expanded": open,
      "data-state": open ? "open" : "closed",
      className: cn(
        "flex w-full items-center justify-between",
        "transition-colors duration-200",
        "focus:outline-none focus:ring-2",
        "focus:outline-none focus:ring-2",
        "focus:ring-primary-500 dark:focus:ring-dark-primary-500",
        "focus:ring-offset-2 focus:ring-offset-surface-50 dark:focus:ring-offset-dark-surface-50",
        className
      ),
      ...props,
      children
    }
  );
};
var CollapsibleContent = ({
  children,
  className
}) => {
  const context = (0, import_react5.useContext)(CollapsibleContext);
  if (!context) {
    throw new Error("CollapsibleContent must be used within a Collapsible");
  }
  const { open } = context;
  return /* @__PURE__ */ (0, import_jsx_runtime9.jsx)(
    "div",
    {
      "data-state": open ? "open" : "closed",
      className: cn(
        "overflow-hidden transition-all duration-300 ease-in-out",
        open ? "animate-in fade-in-0 slide-in-from-top-2" : "animate-out fade-out-0 slide-out-to-top-2",
        !open && "hidden",
        className
      ),
      children
    }
  );
};

// src/components/Droppable.tsx
var import_react6 = require("react");
var import_jsx_runtime10 = require("react/jsx-runtime");
function Droppable({
  droppableId,
  children,
  className,
  isDropDisabled = false
}) {
  const { dragState, updateDrag, endDrag } = useDragDropContext();
  const elementRef = (0, import_react6.useRef)(null);
  const isDraggingOver = !isDropDisabled && dragState.destinationDroppableId === droppableId && dragState.draggedId !== null;
  const handleDragEnter = (e) => {
    e.preventDefault();
    if (!isDropDisabled) {
      updateDrag(droppableId);
    }
  };
  const handleDragOver = (e) => {
    e.preventDefault();
    if (!isDropDisabled) {
      updateDrag(droppableId);
    }
  };
  const handleDragLeave = (e) => {
    e.preventDefault();
    if (!isDropDisabled && !elementRef.current?.contains(e.relatedTarget)) {
      updateDrag(null);
    }
  };
  const handleDrop = (e) => {
    e.preventDefault();
    if (!isDropDisabled) {
      endDrag();
    }
  };
  return /* @__PURE__ */ (0, import_jsx_runtime10.jsx)(
    "div",
    {
      ref: elementRef,
      className: cn(
        "transition-all duration-200 ease-in-out",
        isDraggingOver && "ring-2 ring-accent-500 bg-accent-50 dark:bg-dark-accent-50 rounded-lg",
        className
      ),
      onDragEnter: handleDragEnter,
      onDragOver: handleDragOver,
      onDragLeave: handleDragLeave,
      onDrop: handleDrop,
      children
    }
  );
}

// src/components/TextArea.tsx
var import_react7 = require("react");
var import_jsx_runtime11 = require("react/jsx-runtime");
var Textarea = (0, import_react7.forwardRef)(
  ({ className, error = false, ...props }, ref) => {
    return /* @__PURE__ */ (0, import_jsx_runtime11.jsx)(
      "textarea",
      {
        ref,
        className: cn(
          "bg-surface-50 text-text w-full rounded-lg border px-4 py-2.5 resize-y transition-colors",
          "focus:ring-primary-500 focus:ring-2 focus:ring-offset-2 focus:outline-none",
          "dark:bg-dark-surface-200 dark:text-dark-text dark:focus:ring-dark-primary-500",
          error ? "border-error-300 focus:border-error-500 focus:ring-error-500 dark:border-dark-error-400 dark:focus:border-dark-error-500 dark:focus:ring-dark-error-500" : "border-surface-300 dark:border-dark-surface-600 focus:border-primary-500",
          className
        ),
        ...props
      }
    );
  }
);
Textarea.displayName = "Textarea";

// src/components/Accordion.tsx
var import_lucide_react = require("lucide-react");
var import_react8 = require("react");
var import_jsx_runtime12 = require("react/jsx-runtime");
function Accordion({ title, children, className }) {
  const [isOpen, setIsOpen] = (0, import_react8.useState)(false);
  return /* @__PURE__ */ (0, import_jsx_runtime12.jsxs)(
    "div",
    {
      className: cn(
        "border-secondary-200 dark:border-dark-secondary-300 rounded-lg border",
        className
      ),
      children: [
        /* @__PURE__ */ (0, import_jsx_runtime12.jsxs)(
          Button,
          {
            onClick: () => setIsOpen(!isOpen),
            className: "flex w-full items-center justify-between p-4",
            children: [
              /* @__PURE__ */ (0, import_jsx_runtime12.jsx)(Span, { className: "text-secondary-800 dark:text-dark-secondary-200 text-sm font-medium", children: title }),
              /* @__PURE__ */ (0, import_jsx_runtime12.jsx)(
                import_lucide_react.ChevronDown,
                {
                  className: cn(
                    "text-secondary-400 dark:text-dark-secondary-400 h-5 w-5 transition-transform",
                    isOpen && "rotate-180"
                  )
                }
              )
            ]
          }
        ),
        /* @__PURE__ */ (0, import_jsx_runtime12.jsx)(
          "div",
          {
            className: cn(
              "overflow-hidden transition-all",
              isOpen ? "max-h-[1000px] opacity-100" : "max-h-0 opacity-0"
            ),
            children: /* @__PURE__ */ (0, import_jsx_runtime12.jsx)("div", { className: "p-4 pt-0", children })
          }
        )
      ]
    }
  );
}

// src/components/Badge.tsx
var import_jsx_runtime13 = require("react/jsx-runtime");
function Badge({
  className,
  variant = "default",
  size = "md",
  ...props
}) {
  const baseStyles = cn(
    "inline-flex items-center font-medium rounded-full transition-colors",
    {
      "px-2.5 py-0.5 text-xs": size === "sm",
      "px-3 py-1 text-sm": size === "md"
    }
  );
  const variantStyles = {
    default: cn(
      "bg-[var(--color-primary-100)] text-[var(--color-primary-800)]",
      "dark:bg-[var(--color-dark-primary-900)] dark:text-[var(--color-dark-primary-300)]"
    ),
    primary: cn(
      "bg-[var(--color-primary-100)] text-[var(--color-primary-800)]",
      "dark:bg-[var(--color-dark-primary-900)] dark:text-[var(--color-dark-primary-300)]"
    ),
    accent: cn(
      "bg-[var(--color-accent-100)] text-[var(--color-accent-800)]",
      "dark:bg-[var(--color-dark-accent-900)] dark:text-[var(--color-dark-accent-300)]"
    ),
    success: cn(
      "bg-[var(--color-success-100)] text-[var(--color-success-800)]",
      "dark:bg-[var(--color-dark-success-900)] dark:text-[var(--color-dark-success-300)]"
    ),
    error: cn(
      "bg-[var(--color-error-100)] text-[var(--color-error-800)]",
      "dark:bg-[var(--color-dark-error-900)] dark:text-[var(--color-dark-error-300)]"
    ),
    warning: cn(
      "bg-[var(--color-warning-100)] text-[var(--color-warning-800)]",
      "dark:bg-[var(--color-dark-warning-900)] dark:text-[var(--color-dark-warning-300)]"
    ),
    secondary: cn(
      "bg-[var(--color-secondary-100)] text-[var(--color-secondary-800)]",
      "dark:bg-[var(--color-dark-secondary-900)] dark:text-[var(--color-dark-secondary-300)]"
    ),
    outline: cn(
      "bg-transparent border border-[var(--color-secondary-300)] text-[var(--color-secondary-700)]",
      "dark:border-[var(--color-dark-secondary-300)] dark:text-[var(--color-dark-secondary-300)]"
    )
  };
  return /* @__PURE__ */ (0, import_jsx_runtime13.jsx)(
    "span",
    {
      className: cn(baseStyles, variantStyles[variant], className),
      ...props
    }
  );
}

// src/components/CodeBlock.tsx
var import_react9 = require("react");
var import_jsx_runtime14 = require("react/jsx-runtime");
var CodeBlock = (0, import_react9.forwardRef)(
  function CodeBlock2({ className, ...props }, ref) {
    return /* @__PURE__ */ (0, import_jsx_runtime14.jsx)(
      "pre",
      {
        ref,
        className: cn(
          "font-mono text-xs leading-relaxed",
          "text-text dark:text-dark-text",
          "overflow-auto whitespace-pre",
          className
        ),
        ...props
      }
    );
  }
);

// src/components/Toolbar.tsx
var import_react11 = require("react");

// src/components/Tooltip.tsx
var import_react10 = require("react");
var import_jsx_runtime15 = require("react/jsx-runtime");
function Tooltip({
  content,
  children,
  position = "top",
  className
}) {
  const [show, setShow] = (0, import_react10.useState)(false);
  const tooltipId = (0, import_react10.useId)();
  return /* @__PURE__ */ (0, import_jsx_runtime15.jsxs)("div", { className: "relative inline-block", children: [
    /* @__PURE__ */ (0, import_jsx_runtime15.jsx)(
      "div",
      {
        onMouseEnter: () => setShow(true),
        onMouseLeave: () => setShow(false),
        onFocus: () => setShow(true),
        onBlur: () => setShow(false),
        "aria-describedby": show ? tooltipId : void 0,
        children
      }
    ),
    show && /* @__PURE__ */ (0, import_jsx_runtime15.jsx)(
      "div",
      {
        id: tooltipId,
        role: "tooltip",
        className: cn(
          "absolute z-50 rounded-md px-2 py-1 text-sm",
          "bg-secondary-800 text-surface-50 dark:bg-dark-surface-200 dark:text-dark-text",
          "animate-in fade-in-0 zoom-in-95",
          {
            "bottom-full left-1/2 mb-2 -translate-x-1/2": position === "top",
            "top-full left-1/2 mt-2 -translate-x-1/2": position === "bottom",
            "top-1/2 right-full mr-2 -translate-y-1/2": position === "left",
            "top-1/2 left-full ml-2 -translate-y-1/2": position === "right"
          },
          className
        ),
        children: content
      }
    )
  ] });
}

// src/components/Toolbar.tsx
var import_jsx_runtime16 = require("react/jsx-runtime");
var Toolbar = (0, import_react11.forwardRef)(
  function Toolbar2({ className, ...props }, ref) {
    return /* @__PURE__ */ (0, import_jsx_runtime16.jsx)(
      "div",
      {
        ref,
        role: "toolbar",
        className: cn(
          "bg-surface-50 dark:bg-dark-surface-200",
          "text-text dark:text-dark-text",
          "flex shrink-0 flex-wrap items-center gap-x-3 gap-y-2",
          "border-b border-surface-300 dark:border-dark-surface-400",
          "px-3 py-2",
          className
        ),
        ...props
      }
    );
  }
);
var ToolbarSection = (0, import_react11.forwardRef)(
  function ToolbarSection2({ className, ...props }, ref) {
    return /* @__PURE__ */ (0, import_jsx_runtime16.jsx)(
      "div",
      {
        ref,
        className: cn(
          "flex min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-1.5 sm:gap-x-3",
          className
        ),
        ...props
      }
    );
  }
);
function ToolbarItem({ label, tooltip, children, className, ...props }) {
  return /* @__PURE__ */ (0, import_jsx_runtime16.jsxs)("div", { className: cn("flex min-w-0 items-center gap-2", className), ...props, children: [
    /* @__PURE__ */ (0, import_jsx_runtime16.jsxs)("span", { className: "flex shrink-0 items-center gap-1", children: [
      /* @__PURE__ */ (0, import_jsx_runtime16.jsx)(Span, { variant: "muted", className: "text-xs sm:text-sm", children: label }),
      tooltip && /* @__PURE__ */ (0, import_jsx_runtime16.jsx)(Tooltip, { content: tooltip, position: "top", children: /* @__PURE__ */ (0, import_jsx_runtime16.jsx)(Badge, { variant: "outline", size: "sm", className: "cursor-help px-1", children: "?" }) })
    ] }),
    children
  ] });
}
var ToolbarActions = (0, import_react11.forwardRef)(
  function ToolbarActions2({ className, ...props }, ref) {
    return /* @__PURE__ */ (0, import_jsx_runtime16.jsx)(
      "div",
      {
        ref,
        className: cn("flex shrink-0 items-center gap-1.5", className),
        ...props
      }
    );
  }
);

// src/components/Card.tsx
var import_react12 = require("react");
var import_jsx_runtime17 = require("react/jsx-runtime");
var Card = (0, import_react12.forwardRef)(
  ({ className, variant = "default", layout: layout2 = "default", statusBorder, ...props }, ref) => {
    const statusBorderStyles2 = {
      default: "border-l-4 border-l-border dark:border-l-dark-surface-600",
      success: "border-l-4 border-l-success dark:border-l-dark-success",
      error: "border-l-4 border-l-error dark:border-l-dark-error",
      warning: "border-l-4 border-l-warning dark:border-l-dark-warning",
      info: "border-l-4 border-l-info dark:border-l-dark-info"
    };
    const dottedPatternClasses = "[--dot-size:1px] [--dot-gap:18px] [--dot-color:rgba(0,0,0,0.14)] dark:[--dot-color:rgba(255,255,255,0.14)] [background-image:radial-gradient(var(--dot-color)_var(--dot-size),transparent_var(--dot-size))] [background-size:var(--dot-gap)_var(--dot-gap)] bg-surface-100 dark:bg-dark-surface-700 border-surface-300 dark:border-dark-surface-600";
    return /* @__PURE__ */ (0, import_jsx_runtime17.jsx)(
      "div",
      {
        ref,
        className: cn(
          "rounded-xl border p-6 transition-colors shadow-sm dark:shadow-none",
          "dark:border-dark-surface-700",
          {
            "bg-surface-50 border-surface-200 dark:bg-dark-surface-800": variant === "default",
            "bg-secondary-100 border-secondary-200 dark:bg-dark-surface-600": variant === "filled",
            "bg-surface-100 border-surface-300 dark:bg-dark-surface-900 dark:border-dark-surface-600": variant === "surface",
            "bg-error-50 dark:bg-dark-error-900 text-error dark:text-dark-error": variant === "error",
            "border border-surface-400 dark:border-dark-surface-500": variant === "bordered",
            [dottedPatternClasses]: variant === "dotted"
          },
          {
            "flex items-center justify-between": layout2 === "space-between"
          },
          statusBorder && statusBorderStyles2[statusBorder],
          className
        ),
        ...props
      }
    );
  }
);
Card.displayName = "Card";

// src/components/Checkbox.tsx
var import_react14 = require("react");

// src/components/Inbox.tsx
var import_react13 = require("react");
var import_jsx_runtime18 = require("react/jsx-runtime");
var Inbox = (0, import_react13.forwardRef)(
  ({ className, ...props }, ref) => {
    return /* @__PURE__ */ (0, import_jsx_runtime18.jsx)(
      "input",
      {
        ref,
        className: cn(
          "border-secondary-300 bg-surface-50 text-text rounded-lg border px-4 py-2.5",
          "focus:ring-primary-500 focus:border-transparent focus:ring-2 focus:ring-offset-2",
          "dark:border-dark-secondary-300 dark:bg-dark-surface-50 dark:text-dark-text dark:focus:ring-dark-primary-500",
          className
        ),
        ...props
      }
    );
  }
);
Inbox.displayName = "Inbox";

// src/components/Label.tsx
var import_jsx_runtime19 = require("react/jsx-runtime");
function Label({ className, ...props }) {
  return /* @__PURE__ */ (0, import_jsx_runtime19.jsx)(
    "label",
    {
      className: cn(
        "text-text dark:text-dark-text text-sm font-medium",
        className
      ),
      ...props
    }
  );
}

// src/components/Checkbox.tsx
var import_jsx_runtime20 = require("react/jsx-runtime");
var Checkbox = (0, import_react14.forwardRef)(
  ({ className, indeterminate, label, ...props }, forwardedRef) => {
    const localRef = (0, import_react14.useRef)(null);
    (0, import_react14.useEffect)(() => {
      if (forwardedRef) {
        if (typeof forwardedRef === "function") {
          forwardedRef(localRef.current);
        } else {
          forwardedRef.current = localRef.current;
        }
      }
    }, [forwardedRef]);
    (0, import_react14.useEffect)(() => {
      if (localRef.current) {
        localRef.current.indeterminate = indeterminate ?? false;
      }
    }, [indeterminate]);
    return /* @__PURE__ */ (0, import_jsx_runtime20.jsxs)(Label, { className: "flex items-center gap-2", children: [
      /* @__PURE__ */ (0, import_jsx_runtime20.jsx)(
        Inbox,
        {
          type: "checkbox",
          ref: localRef,
          className: cn(
            "border-secondary-300 text-primary-500 focus:ring-primary-500 h-4 w-4 rounded",
            "dark:border-dark-secondary-400 dark:bg-dark-surface-50 dark:checked:bg-dark-primary-500 dark:focus:ring-dark-primary-500",
            className
          ),
          ...props
        }
      ),
      label && /* @__PURE__ */ (0, import_jsx_runtime20.jsx)("span", { className: "text-secondary-800 dark:text-dark-secondary-300 text-sm", children: label })
    ] });
  }
);
Checkbox.displayName = "Checkbox";

// src/components/Panel.tsx
var import_react15 = require("react");
var import_jsx_runtime21 = require("react/jsx-runtime");
var Panel = (0, import_react15.forwardRef)(
  ({ className, variant = "default", ...props }, ref) => /* @__PURE__ */ (0, import_jsx_runtime21.jsx)(
    "div",
    {
      ref,
      className: cn(
        // Base styles
        "transition-colors",
        // Conditionally remove rounded corners for the topBordered variant
        variant === "body" ? "rounded-none" : "rounded-lg",
        {
          // Variants
          "p-4 inherit bg-inherit text-inherit": variant === "default",
          "p-4 shadow-sm dark:shadow-md": variant === "raised",
          "p-4 border border-surface-300 dark:border-dark-surface-700": variant === "bordered",
          "p-0 border-0 shadow-none": variant === "flat",
          "p-4 bg-error-50 dark:bg-dark-error-900 text-error dark:text-dark-error border border-error-200 dark:border-dark-error-200": variant === "error",
          "p-4 bg-warning-50 dark:bg-dark-warning-50 text-warning-900 dark:text-dark-warning-800 border border-warning-200 dark:border-dark-warning-200": variant === "warning",
          "p-4 bg-info-50 dark:bg-dark-surface-200 text-info-900 dark:text-dark-text border border-info-200 dark:border-dark-surface-500": variant === "info",
          "p-4 bg-gradient-to-br from-primary-600 to-accent-700 !text-white": variant === "gradient",
          "p-4 bg-surface-50 dark:bg-dark-surface-100 border border-surface-200 dark:border-dark-surface-700": variant === "surface",
          "p-4 bg-transparent hover:bg-surface-50 dark:hover:bg-dark-surface-800 border border-surface-100 dark:border-dark-surface-700": variant === "ghost",
          "p-4 border-t border-[var(--color-surface-300)] dark:border-[var(--color-dark-surface-700)]": variant === "body",
          "": variant === "empty"
        },
        className
      ),
      ...props
    }
  )
);

// src/components/Cmdbar.tsx
var import_jsx_runtime22 = require("react/jsx-runtime");

// src/components/CommandPanel.tsx
var import_react16 = require("react");
var import_jsx_runtime23 = require("react/jsx-runtime");
var CommandPanel = (0, import_react16.forwardRef)(
  (props, ref) => {
    const { initialContent = /* @__PURE__ */ (0, import_jsx_runtime23.jsx)(P, { children: "Hi" }), className = "", style = {} } = props;
    const [content, setContent] = (0, import_react16.useState)(initialContent);
    (0, import_react16.useImperativeHandle)(
      ref,
      () => ({
        updateContent(newContent) {
          setContent(newContent);
        },
        resetContent() {
          setContent(initialContent);
        }
      }),
      [initialContent]
    );
    return /* @__PURE__ */ (0, import_jsx_runtime23.jsx)(
      "div",
      {
        className: cn("flex items-center justify-between gap-4 p-4", className),
        style,
        children: content
      }
    );
  }
);
CommandPanel.displayName = "CommandPanel";

// src/components/chat/ApprovalCard.tsx
var import_react18 = require("react");

// src/components/ButtonGroup.tsx
var import_jsx_runtime24 = require("react/jsx-runtime");
function ButtonGroup({
  children,
  className
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime24.jsx)("div", { className: cn("flex gap-2 shrink-0", className), children });
}

// src/components/DiffView.tsx
var import_react17 = require("react");
var import_jsx_runtime25 = require("react/jsx-runtime");
var lineTypeStyles = {
  add: "bg-success-50 dark:bg-dark-success-50 text-success-800 dark:text-dark-success-800",
  remove: "bg-error-50 dark:bg-dark-error-50 text-error-800 dark:text-dark-error-800",
  context: "text-text dark:text-dark-text"
};
var gutterStyles = {
  add: "bg-success-100 dark:bg-dark-success-100 text-success-600 dark:text-dark-success-600",
  remove: "bg-error-100 dark:bg-dark-error-100 text-error-600 dark:text-dark-error-600",
  context: "bg-surface-100 dark:bg-dark-surface-300 text-text-muted dark:text-dark-text-muted"
};
var prefixChar = {
  add: "+",
  remove: "-",
  context: " "
};
var DiffView = (0, import_react17.forwardRef)(
  function DiffView2({ className, filePath, lines, language, ...props }, ref) {
    return /* @__PURE__ */ (0, import_jsx_runtime25.jsxs)(
      "div",
      {
        ref,
        className: cn(
          "overflow-hidden rounded-lg border",
          "border-surface-200 dark:border-dark-surface-500",
          "text-sm",
          className
        ),
        ...props,
        children: [
          /* @__PURE__ */ (0, import_jsx_runtime25.jsxs)(
            "div",
            {
              className: cn(
                "flex items-center gap-2 border-b px-3 py-1.5",
                "bg-surface-100 dark:bg-dark-surface-300",
                "border-surface-200 dark:border-dark-surface-500"
              ),
              children: [
                /* @__PURE__ */ (0, import_jsx_runtime25.jsx)("span", { className: "font-mono text-xs font-medium text-text dark:text-dark-text", children: filePath }),
                language && /* @__PURE__ */ (0, import_jsx_runtime25.jsx)("span", { className: "text-xs text-text-muted dark:text-dark-text-muted", children: language })
              ]
            }
          ),
          /* @__PURE__ */ (0, import_jsx_runtime25.jsx)("div", { className: "overflow-x-auto", children: /* @__PURE__ */ (0, import_jsx_runtime25.jsx)("table", { className: "w-full border-collapse font-mono text-xs leading-5", children: /* @__PURE__ */ (0, import_jsx_runtime25.jsx)("tbody", { children: lines.map((line, i) => /* @__PURE__ */ (0, import_jsx_runtime25.jsxs)("tr", { className: lineTypeStyles[line.type], children: [
            /* @__PURE__ */ (0, import_jsx_runtime25.jsx)(
              "td",
              {
                className: cn(
                  "w-10 select-none px-2 text-right align-top",
                  gutterStyles[line.type]
                ),
                children: line.type !== "add" ? line.oldLineNumber ?? "" : ""
              }
            ),
            /* @__PURE__ */ (0, import_jsx_runtime25.jsx)(
              "td",
              {
                className: cn(
                  "w-10 select-none px-2 text-right align-top",
                  gutterStyles[line.type]
                ),
                children: line.type !== "remove" ? line.newLineNumber ?? "" : ""
              }
            ),
            /* @__PURE__ */ (0, import_jsx_runtime25.jsx)("td", { className: "w-4 select-none px-1 text-center align-top", children: prefixChar[line.type] }),
            /* @__PURE__ */ (0, import_jsx_runtime25.jsx)("td", { className: "whitespace-pre-wrap break-all px-2 align-top", children: line.content })
          ] }, i)) }) }) })
        ]
      }
    );
  }
);

// src/components/KeyValue.tsx
var import_jsx_runtime26 = require("react/jsx-runtime");
var KeyValue = ({
  label,
  value,
  className,
  labelClassName,
  valueClassName
}) => /* @__PURE__ */ (0, import_jsx_runtime26.jsxs)(P, { className: cn("flex gap-2", className), children: [
  /* @__PURE__ */ (0, import_jsx_runtime26.jsx)("span", { className: cn("font-medium shrink-0", labelClassName), children: label }),
  /* @__PURE__ */ (0, import_jsx_runtime26.jsx)("span", { className: cn("truncate", valueClassName), children: value })
] });

// src/components/chat/ApprovalCard.tsx
var import_jsx_runtime27 = require("react/jsx-runtime");
var DEFAULT_LABELS = {
  approvalRequired: "Approval required:",
  showDiff: "\u25B8 Show diff",
  hideDiff: "\u25BE Hide diff",
  approve: "Approve",
  deny: "Deny"
};
function parsePatch(raw) {
  const rawLines = raw.split("\n");
  let filePath = "diff";
  const lines = [];
  let oldLine = 0;
  let newLine = 0;
  for (const text of rawLines) {
    if (text.startsWith("+++ ")) {
      filePath = text.slice(4).replace(/^b\//, "");
      continue;
    }
    if (text.startsWith("--- ")) continue;
    if (text.startsWith("@@ ")) {
      const m = text.match(/@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
      if (m) {
        oldLine = parseInt(m[1], 10);
        newLine = parseInt(m[2], 10);
      }
      lines.push({ type: "context", content: text });
      continue;
    }
    if (text.startsWith("+")) {
      lines.push({ type: "add", content: text.slice(1), newLineNumber: newLine++ });
    } else if (text.startsWith("-")) {
      lines.push({ type: "remove", content: text.slice(1), oldLineNumber: oldLine++ });
    } else {
      lines.push({
        type: "context",
        content: text.startsWith(" ") ? text.slice(1) : text,
        oldLineNumber: oldLine++,
        newLineNumber: newLine++
      });
    }
  }
  return { filePath, lines };
}
function ApprovalCard({ approval, onRespond, labels }) {
  const [inflight, setInflight] = (0, import_react18.useState)(false);
  const [diffExpanded, setDiffExpanded] = (0, import_react18.useState)(false);
  const l = { ...DEFAULT_LABELS, ...labels };
  const handle = (approved) => {
    if (inflight) return;
    setInflight(true);
    onRespond(approved);
  };
  const argEntries = Object.entries(approval.args).filter(
    ([, v]) => v !== "" && v !== null && v !== void 0
  );
  return /* @__PURE__ */ (0, import_jsx_runtime27.jsxs)(Panel, { variant: "warning", children: [
    /* @__PURE__ */ (0, import_jsx_runtime27.jsxs)("div", { className: "flex items-center gap-1.5 text-sm font-semibold", children: [
      "\u26A0 ",
      l.approvalRequired,
      " ",
      /* @__PURE__ */ (0, import_jsx_runtime27.jsxs)(Span, { className: "font-mono text-[0.9em]", children: [
        approval.hookName,
        ".",
        approval.toolName
      ] })
    ] }),
    argEntries.length > 0 && /* @__PURE__ */ (0, import_jsx_runtime27.jsx)("div", { className: "flex flex-col gap-0.5 text-xs", children: argEntries.map(([k, v]) => /* @__PURE__ */ (0, import_jsx_runtime27.jsx)(
      KeyValue,
      {
        label: k,
        value: String(v),
        labelClassName: "text-text-muted dark:text-dark-text-muted pr-3 align-top",
        valueClassName: "break-all font-mono"
      },
      k
    )) }),
    approval.diff && approval.diff !== "(no changes)" && /* @__PURE__ */ (0, import_jsx_runtime27.jsxs)("div", { children: [
      /* @__PURE__ */ (0, import_jsx_runtime27.jsx)(
        Button,
        {
          variant: "ghost",
          size: "sm",
          className: "px-0 text-text-muted dark:text-dark-text-muted",
          onClick: () => setDiffExpanded((e) => !e),
          children: diffExpanded ? l.hideDiff : l.showDiff
        }
      ),
      diffExpanded && (() => {
        const { filePath, lines } = parsePatch(approval.diff);
        return /* @__PURE__ */ (0, import_jsx_runtime27.jsx)(DiffView, { filePath, lines, className: "max-h-80 overflow-auto" });
      })()
    ] }),
    /* @__PURE__ */ (0, import_jsx_runtime27.jsxs)(ButtonGroup, { className: "mt-1", children: [
      /* @__PURE__ */ (0, import_jsx_runtime27.jsx)(Button, { size: "sm", variant: "success", disabled: inflight, onClick: () => handle(true), children: l.approve }),
      /* @__PURE__ */ (0, import_jsx_runtime27.jsx)(Button, { size: "sm", variant: "danger", disabled: inflight, onClick: () => handle(false), children: l.deny })
    ] })
  ] });
}

// src/components/chat/ChatMessage.tsx
var import_react19 = require("react");

// src/components/chat/clipboard.ts
async function copyTextToClipboard(text) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.left = "-9999px";
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch (err) {
    console.warn("Clipboard copy failed:", err);
    return false;
  }
}

// src/components/chat/ChatMessage.tsx
var import_jsx_runtime28 = require("react/jsx-runtime");
function defaultAvatarLetter(role) {
  switch (role) {
    case "user":
      return "U";
    case "system":
      return "S";
    case "tool":
      return "T";
    default:
      return "A";
  }
}
function avatarRingClass(role) {
  switch (role) {
    case "user":
      return "bg-primary-600 text-white";
    case "system":
      return "bg-accent-600 text-white";
    case "tool":
      return "bg-secondary-600 text-white";
    default:
      return "bg-secondary-600 text-white";
  }
}
function roleBadgeVariant(role) {
  if (role === "user") return "primary";
  if (role === "system") return "accent";
  return "secondary";
}
function bubbleBgClass(role) {
  const isUser = role === "user";
  if (isUser) {
    return "bg-surface-100 text-text dark:bg-dark-surface-300 dark:text-dark-text";
  }
  return "bg-surface-50 text-text dark:bg-dark-surface-200 dark:text-dark-text";
}
function transcriptBlockClass(role) {
  switch (role) {
    case "user":
      return "border-l-primary-500 bg-surface-50 text-text shadow-sm dark:border-l-dark-primary-500 dark:bg-dark-surface-300/40 dark:text-dark-text";
    case "system":
      return "border-l-primary-400 bg-surface-50/70 text-text shadow-sm dark:border-l-dark-primary-600 dark:bg-dark-surface-300/30 dark:text-dark-text";
    case "tool":
      return "border-l-secondary-500 bg-surface-50/70 text-text shadow-sm dark:border-l-dark-surface-500 dark:bg-dark-surface-300/30 dark:text-dark-text";
    default:
      return "border-l-secondary-500 bg-surface-50/70 text-text shadow-sm dark:border-l-dark-surface-500 dark:bg-dark-surface-300/30 dark:text-dark-text";
  }
}
function ChatMessage({
  role,
  roleLabel,
  children,
  avatar,
  timestamp,
  timestampTooltip,
  isLatest = false,
  latestLabel,
  highlightLatest = true,
  defaultOpen = true,
  onOpenChange,
  error,
  onRetry,
  retryLabel,
  collapseToggleLabel,
  secondaryActions,
  copyText,
  copyLabel,
  copiedLabel,
  className,
  "aria-label": ariaLabel,
  appearance = "bubble"
}) {
  const [open, setOpen] = (0, import_react19.useState)(defaultOpen);
  const [copied, setCopied] = (0, import_react19.useState)(false);
  const isUser = role === "user";
  const bubbleRing = isLatest && highlightLatest ? "ring-2 ring-surface-300 dark:ring-dark-surface-500" : "";
  const transcriptRing = isLatest && highlightLatest ? "ring-2 ring-surface-300/70 dark:ring-dark-surface-500/60" : "";
  const handleOpenChange = (next) => {
    setOpen(next);
    onOpenChange?.(next);
  };
  const handleCopy = async () => {
    if (!copyText) return;
    const ok = await copyTextToClipboard(copyText);
    if (ok) {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    }
  };
  const collapseLabels = collapseToggleLabel ?? {
    open: "Hide",
    closed: "Show"
  };
  const ts = timestampTooltip ? /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(Tooltip, { content: timestampTooltip, children: /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(
    Span,
    {
      variant: "muted",
      className: "text-secondary-600 dark:text-dark-text-muted text-xs",
      children: timestamp
    }
  ) }) : /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(
    Span,
    {
      variant: "muted",
      className: "text-secondary-600 dark:text-dark-text-muted text-xs",
      children: timestamp
    }
  );
  const articleLabel = ariaLabel ?? (typeof roleLabel === "string" ? roleLabel : "message");
  if (appearance === "transcript") {
    return /* @__PURE__ */ (0, import_jsx_runtime28.jsx)("article", { "aria-label": articleLabel, className: cn("group", className), children: /* @__PURE__ */ (0, import_jsx_runtime28.jsxs)(
      Collapsible,
      {
        open,
        onOpenChange: handleOpenChange,
        className: "flex flex-col gap-1.5",
        children: [
          /* @__PURE__ */ (0, import_jsx_runtime28.jsxs)("div", { className: "flex flex-wrap items-center gap-2", children: [
            /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(Badge, { variant: roleBadgeVariant(role), size: "sm", children: roleLabel }),
            timestamp != null && ts,
            isLatest && latestLabel != null && /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(Badge, { variant: "success", size: "sm", children: latestLabel }),
            /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(CollapsibleTrigger, { asChild: true, children: /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(
              Button,
              {
                variant: "ghost",
                size: "xs",
                className: "h-6 px-2 text-xs",
                type: "button",
                children: open ? collapseLabels.open : collapseLabels.closed
              }
            ) })
          ] }),
          /* @__PURE__ */ (0, import_jsx_runtime28.jsxs)(CollapsibleContent, { children: [
            /* @__PURE__ */ (0, import_jsx_runtime28.jsxs)(
              "div",
              {
                className: cn(
                  "rounded-r-lg border border-l-4 border-surface-200 dark:border-dark-surface-600 py-3 pr-3 pl-4",
                  transcriptBlockClass(role),
                  transcriptRing
                ),
                children: [
                  /* @__PURE__ */ (0, import_jsx_runtime28.jsx)("div", { className: "prose prose-compact dark:prose-invert max-w-none min-w-0", children }),
                  error != null && /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(Panel, { className: "bg-error-50 dark:bg-dark-error-600/30 text-error-800 dark:text-dark-text", children: /* @__PURE__ */ (0, import_jsx_runtime28.jsxs)("div", { className: "flex items-center justify-between gap-2", children: [
                    /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(Span, { className: "text-sm", children: error }),
                    onRetry != null && /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(Button, { variant: "ghost", size: "sm", onClick: onRetry, children: retryLabel ?? "Retry" })
                  ] }) })
                ]
              }
            ),
            /* @__PURE__ */ (0, import_jsx_runtime28.jsxs)("div", { className: "flex flex-wrap items-center gap-2", children: [
              copyText != null && /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(
                Button,
                {
                  variant: "ghost",
                  size: "sm",
                  className: "h-6 text-xs",
                  onClick: () => void handleCopy(),
                  "aria-live": "polite",
                  type: "button",
                  "aria-label": copied ? copiedLabel != null ? String(copiedLabel) : "Copied" : copyLabel != null ? String(copyLabel) : "Copy",
                  children: copied ? copiedLabel ?? "Copied!" : copyLabel ?? "Copy"
                }
              ),
              secondaryActions
            ] })
          ] })
        ]
      }
    ) });
  }
  return /* @__PURE__ */ (0, import_jsx_runtime28.jsx)("article", { "aria-label": articleLabel, className: cn("group", className), children: /* @__PURE__ */ (0, import_jsx_runtime28.jsxs)(
    Collapsible,
    {
      open,
      onOpenChange: handleOpenChange,
      className: cn("flex gap-3", isUser && "flex-row-reverse"),
      children: [
        /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(
          "div",
          {
            className: cn(
              "flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-xs font-semibold",
              avatarRingClass(role)
            ),
            "aria-hidden": true,
            children: avatar ?? defaultAvatarLetter(role)
          }
        ),
        /* @__PURE__ */ (0, import_jsx_runtime28.jsxs)(
          "div",
          {
            className: cn(
              "flex max-w-[85%] flex-col gap-2",
              isUser && "items-end"
            ),
            children: [
              /* @__PURE__ */ (0, import_jsx_runtime28.jsxs)("div", { className: "flex flex-wrap items-center gap-2", children: [
                /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(Badge, { variant: roleBadgeVariant(role), size: "sm", children: roleLabel }),
                timestamp != null && ts,
                isLatest && latestLabel != null && /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(Badge, { variant: "success", size: "sm", children: latestLabel }),
                /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(CollapsibleTrigger, { asChild: true, children: /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(
                  Button,
                  {
                    variant: "ghost",
                    size: "xs",
                    className: "h-6 px-2 text-xs",
                    type: "button",
                    children: open ? collapseLabels.open : collapseLabels.closed
                  }
                ) })
              ] }),
              /* @__PURE__ */ (0, import_jsx_runtime28.jsxs)(CollapsibleContent, { children: [
                /* @__PURE__ */ (0, import_jsx_runtime28.jsxs)(
                  Card,
                  {
                    variant: "surface",
                    className: cn(
                      "border-surface-200 dark:border-dark-surface-600 rounded-xl border p-4 shadow-sm group-hover:shadow-md",
                      bubbleBgClass(role),
                      bubbleRing
                    ),
                    children: [
                      /* @__PURE__ */ (0, import_jsx_runtime28.jsx)("div", { className: "prose prose-compact dark:prose-invert max-w-none", children }),
                      error != null && /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(Panel, { className: "bg-error-50 dark:bg-dark-error-600/30 text-error-800 dark:text-dark-text", children: /* @__PURE__ */ (0, import_jsx_runtime28.jsxs)("div", { className: "flex items-center justify-between gap-2", children: [
                        /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(Span, { className: "text-sm", children: error }),
                        onRetry != null && /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(Button, { variant: "ghost", size: "sm", onClick: onRetry, children: retryLabel ?? "Retry" })
                      ] }) })
                    ]
                  }
                ),
                /* @__PURE__ */ (0, import_jsx_runtime28.jsxs)(
                  "div",
                  {
                    className: cn(
                      "flex items-center gap-2 opacity-0 transition-opacity group-hover:opacity-100",
                      isUser && "flex-row-reverse"
                    ),
                    children: [
                      copyText != null && /* @__PURE__ */ (0, import_jsx_runtime28.jsx)(
                        Button,
                        {
                          variant: "ghost",
                          size: "sm",
                          className: "h-6 text-xs",
                          onClick: () => void handleCopy(),
                          "aria-live": "polite",
                          type: "button",
                          "aria-label": copied ? copiedLabel != null ? String(copiedLabel) : "Copied" : copyLabel != null ? String(copyLabel) : "Copy",
                          children: copied ? copiedLabel ?? "Copied!" : copyLabel ?? "Copy"
                        }
                      ),
                      secondaryActions
                    ]
                  }
                )
              ] })
            ]
          }
        )
      ]
    }
  ) });
}

// src/components/chat/ChatThread.tsx
var import_jsx_runtime29 = require("react/jsx-runtime");
function ChatThread({
  containerRef,
  endRef,
  children,
  className,
  scrollClassName,
  ariaLive = "polite"
}) {
  const liveProps = ariaLive === false ? {} : { role: "log", "aria-live": ariaLive };
  return /* @__PURE__ */ (0, import_jsx_runtime29.jsx)(
    "div",
    {
      className: cn(
        "text-text dark:text-dark-text flex min-h-0 min-w-0 flex-1 flex-col",
        className
      ),
      children: /* @__PURE__ */ (0, import_jsx_runtime29.jsxs)(
        "div",
        {
          ref: containerRef,
          "data-chat-thread": "",
          className: cn(
            "flex-1 space-y-6 overflow-auto p-6",
            scrollClassName
          ),
          ...liveProps,
          children: [
            children,
            /* @__PURE__ */ (0, import_jsx_runtime29.jsx)("div", { ref: endRef, className: "h-4 shrink-0", "aria-hidden": true })
          ]
        }
      )
    }
  );
}

// src/components/chat/useChatScroll.ts
var import_react20 = require("react");
function useChatScroll({
  deps,
  thresholdPx = 160,
  behavior = "smooth"
}) {
  const containerRef = (0, import_react20.useRef)(null);
  const endRef = (0, import_react20.useRef)(null);
  const [isNearBottom, setIsNearBottom] = (0, import_react20.useState)(true);
  const checkNearBottom = (0, import_react20.useCallback)(() => {
    const el = containerRef.current;
    if (!el) return true;
    return el.scrollHeight - el.scrollTop - el.clientHeight < thresholdPx;
  }, [thresholdPx]);
  const scrollToEnd = (0, import_react20.useCallback)(() => {
    endRef.current?.scrollIntoView({ behavior });
  }, [behavior]);
  (0, import_react20.useEffect)(() => {
    const el = containerRef.current;
    if (!el) return;
    const handleScroll = () => {
      setIsNearBottom(checkNearBottom());
    };
    el.addEventListener("scroll", handleScroll, { passive: true });
    return () => el.removeEventListener("scroll", handleScroll);
  }, [checkNearBottom]);
  (0, import_react20.useEffect)(() => {
    if (checkNearBottom()) {
      endRef.current?.scrollIntoView({ behavior });
    }
  }, [thresholdPx, behavior, checkNearBottom, ...deps]);
  return { containerRef, endRef, scrollToEnd, isNearBottom };
}

// src/components/Skeleton.tsx
var import_jsx_runtime30 = require("react/jsx-runtime");
function Skeleton({
  className,
  variant = "line",
  ...props
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime30.jsx)(
    "div",
    {
      className: cn(
        "bg-secondary-100 dark:bg-dark-surface-200 animate-pulse rounded-md",
        variant === "line" ? "h-4 w-full" : "h-8 w-8 rounded-full",
        className
      ),
      ...props
    }
  );
}

// src/components/chat/ChatThreadSkeleton.tsx
var import_jsx_runtime31 = require("react/jsx-runtime");
function ChatThreadSkeleton({
  rows = 5,
  className
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime31.jsx)("div", { className: cn("flex h-full flex-col gap-4 p-6", className), children: Array.from({ length: rows }).map((_, index) => /* @__PURE__ */ (0, import_jsx_runtime31.jsxs)("div", { className: "flex gap-3", children: [
    /* @__PURE__ */ (0, import_jsx_runtime31.jsx)(Skeleton, { variant: "circle" }),
    /* @__PURE__ */ (0, import_jsx_runtime31.jsxs)("div", { className: "flex-1 space-y-2", children: [
      /* @__PURE__ */ (0, import_jsx_runtime31.jsx)(Skeleton, { variant: "line", className: "h-4 w-32" }),
      /* @__PURE__ */ (0, import_jsx_runtime31.jsx)(Skeleton, { variant: "line", className: "h-16 w-full" })
    ] })
  ] }, index)) });
}

// src/components/chat/ChatProcessingBar.tsx
var import_jsx_runtime32 = require("react/jsx-runtime");
function ChatProcessingBar({
  label,
  onStop,
  stopLabel = "Stop",
  className
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime32.jsx)(
    Panel,
    {
      className: cn(
        "bg-surface-100 dark:bg-dark-surface-200 text-text dark:text-dark-text mx-4 mt-4 shrink-0",
        className
      ),
      children: /* @__PURE__ */ (0, import_jsx_runtime32.jsxs)("div", { className: "flex items-center gap-3", children: [
        /* @__PURE__ */ (0, import_jsx_runtime32.jsx)(Spinner, { size: "sm" }),
        /* @__PURE__ */ (0, import_jsx_runtime32.jsx)(Span, { variant: "body", children: label }),
        /* @__PURE__ */ (0, import_jsx_runtime32.jsx)("div", { className: "flex-1" }),
        onStop != null && /* @__PURE__ */ (0, import_jsx_runtime32.jsx)(Button, { size: "sm", variant: "outline", onClick: onStop, type: "button", children: stopLabel })
      ] })
    }
  );
}

// src/components/chat/ChatComposer.tsx
var import_react21 = require("react");

// src/components/chat/composerSoftLimit.ts
var DEFAULT_COMPOSER_SOFT_MAX = 131072;
function isComposerCharCountWarning(length, softMax) {
  return length > softMax * 0.875;
}
function isOverComposerSoftMax(length, softMax) {
  return length > softMax;
}

// src/components/chat/ChatComposer.tsx
var import_jsx_runtime33 = require("react/jsx-runtime");
var baseTextarea = "border rounded-md bg-surface-50 text-text placeholder:text-secondary-400 border-surface-200 dark:bg-dark-surface-600 dark:text-dark-text dark:placeholder:text-dark-secondary-400 dark:border-dark-surface-700";
function ChatComposer({
  value,
  onChange,
  onSubmit,
  placeholder = "",
  isPending = false,
  disabled = false,
  submitLabel = "Send",
  pendingLabel = "Sending\u2026",
  title,
  className,
  variant = "default",
  shell,
  softMax: softMaxProp,
  maxLength: maxLengthLegacy,
  showCharCount = true,
  charCountFormatter = (len, soft) => `${len}/${soft}`,
  canSubmit = true,
  allowEmptyMessage = false,
  footerStart,
  footerEnd,
  charCountTooltip,
  softLimitExceededNote,
  textareaProps
}) {
  const softMax = softMaxProp ?? maxLengthLegacy ?? DEFAULT_COMPOSER_SOFT_MAX;
  const formRef = (0, import_react21.useRef)(null);
  const [isFocused, setIsFocused] = (0, import_react21.useState)(false);
  const {
    onKeyDown: onKeyDownProp,
    className: textareaClassName,
    ...restTextareaProps
  } = textareaProps ?? {};
  const submitDisabled = disabled || isPending || !allowEmptyMessage && !value.trim() || !canSubmit;
  const effectiveShell = shell ?? (variant === "workbench" ? "plain" : "panel");
  const handleKeyDown = (e) => {
    onKeyDownProp?.(e);
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      if (!submitDisabled) {
        formRef.current?.requestSubmit();
      }
    }
  };
  const countStr = charCountFormatter(value.length, softMax);
  const countWarning = isComposerCharCountWarning(value.length, softMax);
  const overSoftMax = isOverComposerSoftMax(value.length, softMax);
  const textareaBlock = /* @__PURE__ */ (0, import_jsx_runtime33.jsxs)("div", { className: "relative flex-1", children: [
    /* @__PURE__ */ (0, import_jsx_runtime33.jsx)(
      Textarea,
      {
        ...restTextareaProps,
        placeholder,
        value,
        onChange: (e) => onChange(e.target.value),
        onFocus: () => setIsFocused(true),
        onBlur: () => setIsFocused(false),
        required: !allowEmptyMessage,
        disabled,
        className: cn(
          baseTextarea,
          variant === "compact" ? "resize-vertical min-h-[60px]" : variant === "workbench" ? "min-h-[180px] max-h-[50vh] resize-y md:min-h-[200px]" : "resize-vertical min-h-[80px]",
          textareaClassName
        ),
        onKeyDown: handleKeyDown
      }
    ),
    showCharCount && /* @__PURE__ */ (0, import_jsx_runtime33.jsx)("div", { className: "absolute right-2 bottom-2 flex items-center gap-2", children: charCountTooltip != null ? /* @__PURE__ */ (0, import_jsx_runtime33.jsx)(Tooltip, { content: charCountTooltip, children: /* @__PURE__ */ (0, import_jsx_runtime33.jsx)(Badge, { variant: countWarning ? "warning" : "outline", size: "sm", children: countStr }) }) : /* @__PURE__ */ (0, import_jsx_runtime33.jsx)(Badge, { variant: countWarning ? "warning" : "outline", size: "sm", children: countStr }) })
  ] });
  const softNoteBlock = overSoftMax && softLimitExceededNote ? /* @__PURE__ */ (0, import_jsx_runtime33.jsx)("p", { className: "text-text-muted dark:text-dark-secondary-400 text-xs", children: softLimitExceededNote }) : null;
  const submitButton = (compactHeight, workbenchTall) => /* @__PURE__ */ (0, import_jsx_runtime33.jsx)(
    Button,
    {
      type: "submit",
      variant: "primary",
      disabled: submitDisabled,
      size: "lg",
      className: cn(
        compactHeight && "h-[60px]",
        workbenchTall && "min-h-[3rem] self-end"
      ),
      children: isPending ? /* @__PURE__ */ (0, import_jsx_runtime33.jsxs)(import_jsx_runtime33.Fragment, { children: [
        /* @__PURE__ */ (0, import_jsx_runtime33.jsx)(Spinner, { size: "sm", className: "mr-2" }),
        pendingLabel
      ] }) : submitLabel
    }
  );
  const handleFormSubmit = (e) => {
    e.preventDefault();
    onSubmit(e);
  };
  if (variant === "compact") {
    return /* @__PURE__ */ (0, import_jsx_runtime33.jsx)("div", { className, children: /* @__PURE__ */ (0, import_jsx_runtime33.jsxs)(
      "form",
      {
        ref: formRef,
        onSubmit: handleFormSubmit,
        className: "flex items-start gap-2",
        children: [
          textareaBlock,
          submitButton(true)
        ]
      }
    ) });
  }
  const formInner = /* @__PURE__ */ (0, import_jsx_runtime33.jsxs)("form", { ref: formRef, onSubmit: handleFormSubmit, className: "space-y-6", children: [
    title != null && title !== "" && variant !== "workbench" && /* @__PURE__ */ (0, import_jsx_runtime33.jsx)(H2, { className: "text-text dark:text-dark-text text-2xl font-semibold", children: title }),
    /* @__PURE__ */ (0, import_jsx_runtime33.jsx)("div", { className: "space-y-4", children: /* @__PURE__ */ (0, import_jsx_runtime33.jsxs)("div", { className: "space-y-3", children: [
      /* @__PURE__ */ (0, import_jsx_runtime33.jsx)("div", { className: "flex gap-2", children: textareaBlock }),
      /* @__PURE__ */ (0, import_jsx_runtime33.jsxs)("div", { className: "flex items-center justify-between gap-2", children: [
        /* @__PURE__ */ (0, import_jsx_runtime33.jsxs)("div", { className: "flex min-w-0 flex-1 flex-wrap items-center gap-2", children: [
          footerStart,
          footerEnd
        ] }),
        submitButton(false, variant === "workbench")
      ] }),
      softNoteBlock
    ] }) })
  ] });
  if (variant === "workbench" && effectiveShell === "plain") {
    return /* @__PURE__ */ (0, import_jsx_runtime33.jsx)(
      "div",
      {
        className: cn(
          "border-surface-200 dark:border-dark-surface-600 bg-surface-50/80 dark:bg-dark-surface-100/80 border-t px-3 py-3 transition-all duration-200 sm:px-4",
          isFocused && "ring-primary-100 dark:ring-dark-primary-500 ring-2 ring-inset",
          className
        ),
        children: formInner
      }
    );
  }
  return /* @__PURE__ */ (0, import_jsx_runtime33.jsx)(
    Panel,
    {
      variant: "default",
      className: cn(
        "transition-all duration-200",
        isFocused && "ring-primary-100 dark:ring-dark-primary-500 ring-2",
        className
      ),
      children: formInner
    }
  );
}

// src/components/chat/ChatTypingIndicator.tsx
var import_jsx_runtime34 = require("react/jsx-runtime");
function ChatTypingIndicator({
  className,
  "aria-label": ariaLabel = "Typing"
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime34.jsx)(
    "div",
    {
      className: cn("flex items-center gap-1.5 px-2 py-1", className),
      role: "status",
      "aria-label": ariaLabel,
      children: [0, 1, 2].map((i) => /* @__PURE__ */ (0, import_jsx_runtime34.jsx)(
        "span",
        {
          className: "bg-secondary-400 dark:bg-dark-secondary-500 h-2 w-2 animate-pulse rounded-full",
          style: { animationDelay: `${i * 180}ms` }
        },
        i
      ))
    }
  );
}

// src/components/chat/ChatDateSeparator.tsx
var import_jsx_runtime35 = require("react/jsx-runtime");
function ChatDateSeparator({
  label,
  className
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime35.jsxs)(
    "div",
    {
      className: cn("flex items-center gap-3 py-2", className),
      role: "separator",
      "aria-label": label,
      children: [
        /* @__PURE__ */ (0, import_jsx_runtime35.jsx)("div", { className: "bg-surface-300 dark:bg-dark-surface-600 h-px flex-1" }),
        /* @__PURE__ */ (0, import_jsx_runtime35.jsx)("span", { className: "text-text-muted dark:text-dark-text-muted shrink-0 text-xs font-medium", children: label }),
        /* @__PURE__ */ (0, import_jsx_runtime35.jsx)("div", { className: "bg-surface-300 dark:bg-dark-surface-600 h-px flex-1" })
      ]
    }
  );
}

// src/components/chat/ChatScrollToLatest.tsx
var import_lucide_react2 = require("lucide-react");
var import_jsx_runtime36 = require("react/jsx-runtime");
function ChatScrollToLatest({
  visible,
  onClick,
  label,
  className
}) {
  if (!visible) return null;
  return /* @__PURE__ */ (0, import_jsx_runtime36.jsx)(
    "div",
    {
      className: cn(
        "pointer-events-none absolute inset-x-0 bottom-4 flex justify-center",
        className
      ),
      children: /* @__PURE__ */ (0, import_jsx_runtime36.jsxs)(
        Button,
        {
          variant: "secondary",
          size: "sm",
          onClick,
          className: "pointer-events-auto shadow-lg",
          "aria-label": label,
          children: [
            /* @__PURE__ */ (0, import_jsx_runtime36.jsx)(import_lucide_react2.ArrowDown, { className: "mr-1.5 h-3.5 w-3.5" }),
            label
          ]
        }
      )
    }
  );
}

// src/components/chat/chatTranscript.tsx
var import_jsx_runtime37 = require("react/jsx-runtime");
var chatTranscriptMarkdownComponents = {
  pre: (props) => {
    return /* @__PURE__ */ (0, import_jsx_runtime37.jsx)(
      "pre",
      {
        className: "bg-surface-200 text-text dark:bg-dark-surface-700 dark:text-dark-text overflow-auto rounded-lg p-3 text-sm sm:p-4",
        ...props
      }
    );
  },
  code: (props) => {
    const { className, children, node, ...rest } = props;
    const match = /language-(\w+)/.exec(className || "");
    if (match || node && node.parent && node.parent.type === "element" && node.parent.tagName === "pre") {
      return /* @__PURE__ */ (0, import_jsx_runtime37.jsx)("code", { className, ...rest, children });
    }
    return /* @__PURE__ */ (0, import_jsx_runtime37.jsx)(
      "code",
      {
        className: cn(
          "bg-surface-200 text-text dark:bg-dark-surface-700 dark:text-dark-text rounded px-1.5 py-0.5 font-mono text-xs",
          className
        ),
        ...rest,
        children
      }
    );
  },
  blockquote: ({ children, ...props }) => /* @__PURE__ */ (0, import_jsx_runtime37.jsx)(
    "blockquote",
    {
      className: "border-primary-400 dark:border-dark-primary-500 bg-surface-50/50 text-text dark:bg-dark-surface-300/20 dark:text-dark-text rounded-r-lg border-l-4 py-2 pl-4",
      ...props,
      children
    }
  )
};
function ChatStreamThinkingBox({
  className,
  children
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime37.jsx)(
    "div",
    {
      className: cn(
        "border-primary-300/50 bg-surface-50/60 dark:border-dark-primary-600/40 dark:bg-dark-surface-300/30 text-text-muted dark:text-dark-text-muted max-h-48 overflow-auto rounded-md border px-3 py-2 font-mono text-xs whitespace-pre-wrap",
        className
      ),
      children
    }
  );
}
function ChatTranscriptStreamingPlaceholder({ children }) {
  return /* @__PURE__ */ (0, import_jsx_runtime37.jsx)("p", { className: "text-text-muted dark:text-dark-text-muted text-sm italic", children });
}
function ChatStreamingCaret({ className }) {
  return /* @__PURE__ */ (0, import_jsx_runtime37.jsx)(
    "span",
    {
      className: cn(
        "bg-primary-500 ml-0.5 inline-block h-3 w-1.5 animate-pulse rounded-sm align-middle",
        className
      ),
      "aria-hidden": true
    }
  );
}

// src/components/chat/TranscriptEmbedCard.tsx
var import_jsx_runtime38 = require("react/jsx-runtime");
function ToggleGlyph() {
  const { open } = useCollapsibleContext();
  return /* @__PURE__ */ (0, import_jsx_runtime38.jsx)("span", { "aria-hidden": true, className: "text-text-muted dark:text-dark-text-muted shrink-0 tabular-nums", children: open ? "\u2212" : "+" });
}
function TranscriptEmbedCard({
  title,
  headerRight,
  children,
  className,
  defaultOpen = false,
  open,
  onOpenChange
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime38.jsxs)(
    Collapsible,
    {
      open,
      onOpenChange,
      defaultOpen,
      className: cn(
        "border-surface-300 dark:border-dark-surface-400 bg-surface-50 dark:bg-dark-surface-200 w-full overflow-hidden rounded-lg border",
        className
      ),
      children: [
        /* @__PURE__ */ (0, import_jsx_runtime38.jsxs)(
          CollapsibleTrigger,
          {
            className: cn(
              "h-auto min-h-0 w-full justify-between gap-2 rounded-none border-0 bg-transparent px-3 py-2.5 font-normal shadow-none ring-0 ring-offset-0",
              "hover:bg-surface-100/80 dark:hover:bg-dark-surface-100/80"
            ),
            children: [
              /* @__PURE__ */ (0, import_jsx_runtime38.jsxs)("span", { className: "flex min-w-0 flex-1 items-center justify-between gap-2 text-left", children: [
                /* @__PURE__ */ (0, import_jsx_runtime38.jsx)(Span, { variant: "body", className: "truncate font-medium", children: title }),
                headerRight ? /* @__PURE__ */ (0, import_jsx_runtime38.jsx)(Span, { variant: "muted", className: "shrink-0 text-xs", children: headerRight }) : null
              ] }),
              /* @__PURE__ */ (0, import_jsx_runtime38.jsx)(ToggleGlyph, {})
            ]
          }
        ),
        /* @__PURE__ */ (0, import_jsx_runtime38.jsx)(CollapsibleContent, { className: "border-surface-300 dark:border-dark-surface-400 border-t px-2 pb-2 pt-2", children })
      ]
    }
  );
}

// src/components/Dialog.tsx
var import_lucide_react3 = require("lucide-react");
var import_jsx_runtime39 = require("react/jsx-runtime");
function Dialog({
  open,
  onClose,
  title,
  children,
  className
}) {
  if (!open) return null;
  return /* @__PURE__ */ (0, import_jsx_runtime39.jsx)("div", { className: "fixed inset-0 z-50 bg-black/50 backdrop-blur-sm", onClick: onClose, children: /* @__PURE__ */ (0, import_jsx_runtime39.jsx)("div", { className: "fixed top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2", onClick: (e) => e.stopPropagation(), children: /* @__PURE__ */ (0, import_jsx_runtime39.jsxs)(Card, { className: cn("w-[400px]", className), children: [
    /* @__PURE__ */ (0, import_jsx_runtime39.jsxs)("div", { className: "mb-4 flex items-center justify-between", children: [
      /* @__PURE__ */ (0, import_jsx_runtime39.jsx)(H3, { className: "text-primary-600 dark:text-dark-primary-500 text-lg font-semibold", children: title }),
      /* @__PURE__ */ (0, import_jsx_runtime39.jsx)(
        Button,
        {
          onClick: onClose,
          className: "text-secondary-500 hover:bg-secondary-100 dark:text-dark-secondary-400 dark:hover:bg-dark-surface-200 rounded-sm p-1",
          children: /* @__PURE__ */ (0, import_jsx_runtime39.jsx)(import_lucide_react3.X, { className: "h-5 w-5 dark:text-dark-secondary-400" })
        }
      )
    ] }),
    children
  ] }) }) });
}

// src/components/Dropdown.tsx
var import_react22 = __toESM(require("react"));
var import_lucide_react4 = require("lucide-react");
var import_jsx_runtime40 = require("react/jsx-runtime");
function Dropdown({
  isOpen: controlledOpen,
  onToggle,
  trigger,
  options,
  value,
  onChange,
  children,
  contentClassName,
  className
}) {
  const [internalOpen, setInternalOpen] = (0, import_react22.useState)(false);
  const dropdownRef = (0, import_react22.useRef)(null);
  const isControlled = controlledOpen !== void 0;
  const isOpen = isControlled ? controlledOpen : internalOpen;
  const toggle = () => {
    if (!isControlled) setInternalOpen(!isOpen);
    onToggle?.(!isOpen);
  };
  const close = () => {
    if (!isControlled) setInternalOpen(false);
    onToggle?.(false);
  };
  const closeRef = (0, import_react22.useRef)(close);
  closeRef.current = close;
  (0, import_react22.useEffect)(() => {
    const handleClickOutside = (event) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target)) {
        closeRef.current();
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);
  const triggerElement = trigger ? import_react22.default.cloneElement(trigger, {
    onClick: (e) => {
      e.stopPropagation();
      trigger.props.onClick?.(e);
      toggle();
    },
    "aria-haspopup": true,
    "aria-expanded": isOpen
  }) : options ? /* @__PURE__ */ (0, import_jsx_runtime40.jsxs)(
    Button,
    {
      onClick: toggle,
      "aria-haspopup": "true",
      "aria-expanded": isOpen,
      className: cn(
        "border-secondary-300 bg-surface-50 flex items-center justify-between rounded-lg border px-4 py-2.5",
        "focus:ring-primary-500 focus:ring-2 focus:ring-offset-2",
        "dark:border-dark-secondary-300 dark:bg-dark-surface-50"
      ),
      children: [
        /* @__PURE__ */ (0, import_jsx_runtime40.jsx)("span", { className: "text-text dark:text-dark-text", children: options.find((opt) => opt.value === value)?.label || "Select" }),
        /* @__PURE__ */ (0, import_jsx_runtime40.jsx)(import_lucide_react4.ChevronDown, { className: "text-secondary-400 dark:text-dark-secondary-400 h-5 w-5" })
      ]
    }
  ) : null;
  const content = children ? children : options ? options.map((option) => /* @__PURE__ */ (0, import_jsx_runtime40.jsx)(
    Button,
    {
      role: "menuitem",
      onClick: () => {
        onChange?.(option.value);
        close();
      },
      className: cn(
        "text-text hover:bg-secondary-100 w-full px-4 py-2 text-left",
        "dark:text-dark-text dark:hover:bg-dark-surface-100",
        option.value === value && "bg-primary-50 dark:bg-dark-primary-900"
      ),
      children: option.label
    },
    option.value
  )) : null;
  return /* @__PURE__ */ (0, import_jsx_runtime40.jsxs)("div", { className: cn("relative", className), ref: dropdownRef, children: [
    triggerElement,
    isOpen && /* @__PURE__ */ (0, import_jsx_runtime40.jsx)(
      "div",
      {
        className: cn(
          "absolute z-50 mt-2 w-full rounded-lg border bg-surface-50 dark:bg-dark-surface-200 shadow-lg",
          contentClassName
        ),
        role: "menu",
        "aria-hidden": !isOpen,
        children: content
      }
    )
  ] });
}

// src/components/Section.tsx
var import_jsx_runtime41 = require("react/jsx-runtime");
function Section({
  title,
  description,
  className,
  children,
  variant = "bordered",
  ...props
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime41.jsxs)(Panel, { variant, className: cn(className), ...props, children: [
    title && /* @__PURE__ */ (0, import_jsx_runtime41.jsx)(H2, { children: title }),
    description && /* @__PURE__ */ (0, import_jsx_runtime41.jsx)(P, { children: description }),
    /* @__PURE__ */ (0, import_jsx_runtime41.jsx)("section", { children })
  ] });
}

// src/components/EmptyState.tsx
var import_jsx_runtime42 = require("react/jsx-runtime");
function EmptyState({
  title,
  subtitle,
  description,
  icon,
  className,
  orientation = "vertical",
  iconSize = "md",
  variant = "default"
}) {
  const variantStyles = {
    default: cn("text-text dark:text-dark-text"),
    info: cn(
      "text-text dark:text-dark-text",
      "bg-surface-50 dark:bg-dark-surface-50"
    ),
    success: cn(
      "text-[var(--color-success-800)] dark:text-[var(--color-dark-success-200)]",
      "bg-[var(--color-success-50)] dark:bg-[var(--color-dark-success-900)]"
    ),
    warning: cn(
      "text-[var(--color-warning-800)] dark:text-[var(--color-dark-warning-200)]",
      "bg-[var(--color-warning-50)] dark:bg-[var(--color-dark-warning-900)]"
    ),
    error: cn(
      "text-[var(--color-error-800)] dark:text-[var(--color-dark-error-200)]",
      "bg-[var(--color-error-50)] dark:bg-[var(--color-dark-error-900)]"
    )
  };
  return /* @__PURE__ */ (0, import_jsx_runtime42.jsxs)(
    Section,
    {
      title,
      className: cn(
        "p-8 rounded-xl",
        orientation === "horizontal" ? "flex items-center gap-6 text-left" : "text-center",
        variantStyles[variant],
        className
      ),
      children: [
        icon && /* @__PURE__ */ (0, import_jsx_runtime42.jsx)(
          "div",
          {
            className: cn(
              "text-primary dark:text-dark-primary",
              orientation === "horizontal" ? "flex-shrink-0" : "mx-auto",
              {
                "text-3xl": iconSize === "lg",
                "text-2xl": iconSize === "md",
                "text-xl": iconSize === "sm"
              }
            ),
            children: icon
          }
        ),
        subtitle && /* @__PURE__ */ (0, import_jsx_runtime42.jsx)(
          P,
          {
            variant: "lead",
            className: "text-text-muted dark:text-dark-text-muted",
            children: subtitle
          }
        ),
        /* @__PURE__ */ (0, import_jsx_runtime42.jsx)(P, { variant: orientation === "horizontal" ? void 0 : "cardSubtitle", children: description })
      ]
    }
  );
}

// src/components/Form.tsx
var import_jsx_runtime43 = require("react/jsx-runtime");
function Form({
  title,
  onSubmit,
  error,
  onError,
  actions,
  variant = "default",
  children,
  className
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime43.jsx)(Panel, { variant, className, children: /* @__PURE__ */ (0, import_jsx_runtime43.jsxs)(
    "form",
    {
      onSubmit: (e) => {
        e.preventDefault();
        try {
          onSubmit(e);
        } catch (err) {
          onError?.(err instanceof Error ? err.message : String(err));
        }
      },
      className: "space-y-6",
      children: [
        title && /* @__PURE__ */ (0, import_jsx_runtime43.jsx)(H2, { className: "text-text dark:text-dark-text text-2xl font-semibold", children: title }),
        /* @__PURE__ */ (0, import_jsx_runtime43.jsx)("div", { className: "space-y-4", children }),
        error && /* @__PURE__ */ (0, import_jsx_runtime43.jsx)(Panel, { variant: "error", className: "text-sm font-medium", children: error }),
        actions && /* @__PURE__ */ (0, import_jsx_runtime43.jsx)("div", { className: "flex gap-4", children: actions })
      ]
    }
  ) });
}

// src/components/FormField.tsx
var import_lucide_react5 = require("lucide-react");
var import_jsx_runtime44 = require("react/jsx-runtime");
function FormField({
  label,
  required,
  error,
  description,
  tooltip,
  children,
  className
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime44.jsxs)("div", { className: cn("space-y-1.5", className), children: [
    /* @__PURE__ */ (0, import_jsx_runtime44.jsxs)("div", { className: "flex items-baseline justify-between", children: [
      /* @__PURE__ */ (0, import_jsx_runtime44.jsxs)("div", { className: "flex items-center gap-1", children: [
        /* @__PURE__ */ (0, import_jsx_runtime44.jsxs)(Label, { className: "text-sm font-medium", children: [
          label,
          required && /* @__PURE__ */ (0, import_jsx_runtime44.jsx)("span", { className: "text-error dark:text-dark-error", children: "*" })
        ] }),
        tooltip && /* @__PURE__ */ (0, import_jsx_runtime44.jsx)(Tooltip, { content: tooltip, children: /* @__PURE__ */ (0, import_jsx_runtime44.jsx)(import_lucide_react5.HelpCircle, { className: "h-4 w-4 text-text-muted dark:text-dark-text-muted cursor-help" }) })
      ] }),
      description && /* @__PURE__ */ (0, import_jsx_runtime44.jsx)("span", { className: "text-xs text-text-muted dark:text-dark-text-muted", children: description })
    ] }),
    children,
    error && /* @__PURE__ */ (0, import_jsx_runtime44.jsx)(P, { className: "text-xs text-error dark:text-dark-error flex items-center gap-1", children: error })
  ] });
}

// src/components/InlineNotice.tsx
var import_react23 = require("react");
var import_jsx_runtime45 = require("react/jsx-runtime");
var variantClasses = {
  warning: "bg-warning-50 dark:bg-dark-surface-300 text-warning-900 dark:text-dark-text border-warning-200 dark:border-dark-surface-500 shrink-0 border-b px-3 py-1.5 text-xs",
  info: "border-surface-300 bg-surface-100 text-text dark:border-dark-surface-500 dark:bg-dark-surface-200 dark:text-dark-text shrink-0 rounded-lg border px-3 py-2 text-sm whitespace-pre-wrap",
  error: "bg-error-50 dark:bg-dark-error-100 text-error-800 dark:text-dark-text border-error-200 dark:border-dark-surface-500 shrink-0 rounded-lg border px-3 py-2 text-sm",
  errorSoft: "bg-error-50 dark:bg-dark-error-400 text-error-800 dark:text-dark-text shrink-0 rounded-lg border border-error-200 p-4 dark:border-dark-surface-600"
};
var InlineNotice = (0, import_react23.forwardRef)(function InlineNotice2({ className, variant = "info", onDismiss, children, ...props }, ref) {
  return /* @__PURE__ */ (0, import_jsx_runtime45.jsx)("div", { ref, className: cn(variantClasses[variant], className), ...props, children: onDismiss ? /* @__PURE__ */ (0, import_jsx_runtime45.jsxs)("div", { className: "flex items-center justify-between gap-2", children: [
    /* @__PURE__ */ (0, import_jsx_runtime45.jsx)("div", { className: "min-w-0 flex-1", children }),
    /* @__PURE__ */ (0, import_jsx_runtime45.jsx)(
      "button",
      {
        type: "button",
        onClick: onDismiss,
        className: "text-current opacity-60 hover:opacity-100 shrink-0 px-1 text-lg leading-none",
        "aria-label": "Dismiss",
        children: "\xD7"
      }
    )
  ] }) : children });
});

// src/components/InsetPanel.tsx
var import_react24 = require("react");
var import_jsx_runtime46 = require("react/jsx-runtime");
var InsetPanel = (0, import_react24.forwardRef)(function InsetPanel2({ className, tone = "default", ...props }, ref) {
  return /* @__PURE__ */ (0, import_jsx_runtime46.jsx)(
    "div",
    {
      ref,
      className: cn(
        "border-surface-300 dark:border-dark-surface-400",
        tone === "default" && "bg-surface-50 dark:bg-dark-surface-200 flex min-h-0 flex-col overflow-hidden rounded-lg border",
        tone === "muted" && "bg-surface-100 dark:bg-dark-surface-300 rounded-lg border",
        tone === "strip" && "bg-surface-100 dark:bg-dark-surface-300 flex shrink-0 flex-col border-b",
        tone === "section" && "bg-surface-100 dark:bg-dark-surface-300 flex min-h-0 shrink-0 flex-col border-b",
        className
      ),
      ...props
    }
  );
});
var InsetPanelHeader = (0, import_react24.forwardRef)(
  function InsetPanelHeader2({ className, density = "compact", ...props }, ref) {
    return /* @__PURE__ */ (0, import_jsx_runtime46.jsx)(
      "div",
      {
        ref,
        className: cn(
          "border-surface-300 dark:border-dark-surface-400 shrink-0 border-b px-3",
          density === "compact" ? "py-1.5" : "py-2",
          className
        ),
        ...props
      }
    );
  }
);
var InsetPanelBody = (0, import_react24.forwardRef)(
  function InsetPanelBody2({ className, ...props }, ref) {
    return /* @__PURE__ */ (0, import_jsx_runtime46.jsx)(
      "div",
      {
        ref,
        className: cn("min-h-0 flex-1 overflow-hidden px-2 pb-2", className),
        ...props
      }
    );
  }
);

// src/components/Pagination.tsx
var import_lucide_react6 = require("lucide-react");
var import_jsx_runtime47 = require("react/jsx-runtime");
function Pagination({
  currentPage,
  totalPages,
  onPageChange,
  className
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime47.jsxs)(
    "div",
    {
      className: cn("flex items-center justify-between px-4 py-3", className),
      children: [
        /* @__PURE__ */ (0, import_jsx_runtime47.jsxs)(
          Button,
          {
            onClick: () => onPageChange(Math.max(1, currentPage - 1)),
            disabled: currentPage === 1,
            className: cn(
              "flex items-center gap-1 rounded-lg px-3 py-1.5",
              "text-secondary-600 hover:bg-secondary-100",
              "dark:text-dark-secondary-400 dark:hover:bg-dark-surface-200",
              "disabled:opacity-50 disabled:hover:bg-transparent"
            ),
            children: [
              /* @__PURE__ */ (0, import_jsx_runtime47.jsx)(import_lucide_react6.ChevronLeft, { className: "h-4 w-4 dark:text-dark-secondary-400" }),
              "Previous"
            ]
          }
        ),
        /* @__PURE__ */ (0, import_jsx_runtime47.jsxs)(Span, { className: "text-secondary-600 dark:text-dark-secondary-400 text-sm", children: [
          "Page ",
          currentPage,
          " of ",
          totalPages
        ] }),
        /* @__PURE__ */ (0, import_jsx_runtime47.jsxs)(
          Button,
          {
            onClick: () => onPageChange(Math.min(totalPages, currentPage + 1)),
            disabled: currentPage === totalPages,
            className: cn(
              "flex items-center gap-1 rounded-lg px-3 py-1.5",
              "text-secondary-600 hover:bg-secondary-100",
              "dark:text-dark-secondary-400 dark:hover:bg-dark-surface-200",
              "disabled:opacity-50 disabled:hover:bg-transparent"
            ),
            children: [
              "Next",
              /* @__PURE__ */ (0, import_jsx_runtime47.jsx)(import_lucide_react6.ChevronRight, { className: "h-4 w-4 dark:text-dark-secondary-400" })
            ]
          }
        )
      ]
    }
  );
}

// src/components/ProgressBar.tsx
var import_jsx_runtime48 = require("react/jsx-runtime");
function ProgressBar({
  value,
  palette = "neutral",
  className
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime48.jsx)(
    "div",
    {
      className: cn(
        "h-2 bg-surface-200 rounded-full overflow-hidden",
        className
      ),
      children: /* @__PURE__ */ (0, import_jsx_runtime48.jsx)(
        "div",
        {
          className: cn("h-full transition-all duration-500 ease-out", {
            "bg-surface-300": palette === "neutral",
            "bg-green-500 dark:bg-dark-success-500": palette === "success",
            "bg-yellow-500 dark:bg-dark-warning-500": palette === "warning",
            "bg-primary-500": palette === "primary",
            "bg-red-500 dark:bg-dark-error-500": palette === "error"
          }),
          style: { width: `${value}%` }
        }
      )
    }
  );
}

// src/components/SearchBar.tsx
var import_lucide_react7 = require("lucide-react");
var import_react25 = require("react");
var import_jsx_runtime49 = require("react/jsx-runtime");
var SearchBar = (0, import_react25.forwardRef)(
  ({ className, value, onClear, ...props }, ref) => {
    return /* @__PURE__ */ (0, import_jsx_runtime49.jsxs)("div", { className: "relative", children: [
      /* @__PURE__ */ (0, import_jsx_runtime49.jsx)(import_lucide_react7.Search, { className: "text-secondary-400 dark:text-dark-secondary-400 absolute top-1/2 left-3 h-5 w-5 -translate-y-1/2" }),
      /* @__PURE__ */ (0, import_jsx_runtime49.jsx)(
        Inbox,
        {
          ref,
          value,
          className: cn(
            "border-secondary-300 bg-surface-50 w-full rounded-lg border py-2.5 pr-8 pl-10",
            "focus:ring-primary-500 focus:ring-2 focus:ring-offset-2",
            "dark:border-dark-secondary-300 dark:bg-dark-surface-50 dark:text-dark-text",
            className
          ),
          ...props
        }
      ),
      value && /* @__PURE__ */ (0, import_jsx_runtime49.jsx)(
        Button,
        {
          onClick: onClear,
          className: "absolute top-1/2 right-3 -translate-y-1/2 text-secondary-400 hover:text-secondary-600 dark:text-dark-secondary-400 dark:hover:text-dark-secondary-600",
          children: /* @__PURE__ */ (0, import_jsx_runtime49.jsx)(import_lucide_react7.X, { className: "h-5 w-5" })
        }
      )
    ] });
  }
);
SearchBar.displayName = "SearchBar";

// src/components/Select.tsx
var import_react26 = require("react");
var import_jsx_runtime50 = require("react/jsx-runtime");
var Select = (0, import_react26.forwardRef)(
  ({ className, options, placeholder, ...props }, ref) => /* @__PURE__ */ (0, import_jsx_runtime50.jsxs)(
    "select",
    {
      ref,
      className: cn(
        "rounded-lg border h-9 px-3 py-1 text-sm",
        "text-text dark:text-dark-text",
        "bg-surface-50 dark:bg-dark-surface-50",
        "border-surface-300 dark:border-dark-surface-600",
        "focus:ring-2 focus:outline-none",
        "focus:ring-primary-500 dark:focus:ring-dark-primary-500",
        "focus:border-transparent",
        "focus:ring-offset-2 focus:ring-offset-surface-50 dark:focus:ring-offset-dark-surface-100",
        className
      ),
      ...props,
      children: [
        placeholder && /* @__PURE__ */ (0, import_jsx_runtime50.jsx)(SelectOption, { value: "", disabled: true, hidden: true, children: placeholder }),
        options.map((option) => /* @__PURE__ */ (0, import_jsx_runtime50.jsx)(SelectOption, { value: option.value, children: option.label }, option.value))
      ]
    }
  )
);
Select.displayName = "Select";
var SelectOption = (0, import_react26.forwardRef)(
  ({ className, ...props }, ref) => /* @__PURE__ */ (0, import_jsx_runtime50.jsx)(
    "option",
    {
      ref,
      className: cn(
        "bg-surface-50 text-text dark:bg-dark-surface-50 dark:text-dark-text",
        className
      ),
      ...props
    }
  )
);
SelectOption.displayName = "SelectOption";

// src/components/SidebarToggle.tsx
var import_lucide_react8 = require("lucide-react");
var import_jsx_runtime51 = require("react/jsx-runtime");
function SidebarToggle({ isOpen, onToggle }) {
  return /* @__PURE__ */ (0, import_jsx_runtime51.jsx)(
    Button,
    {
      variant: "ghost",
      size: "icon",
      onClick: onToggle,
      "aria-label": "Toggle sidebar",
      children: isOpen ? /* @__PURE__ */ (0, import_jsx_runtime51.jsx)(import_lucide_react8.X, { className: "size-6" }) : /* @__PURE__ */ (0, import_jsx_runtime51.jsx)(import_lucide_react8.Menu, { className: "size-6" })
    }
  );
}

// src/components/SidePanel.tsx
var import_react27 = require("react");
var import_jsx_runtime52 = require("react/jsx-runtime");
var SidePanelColumn = (0, import_react27.forwardRef)(
  function SidePanelColumn2({ className, side = "right", ...props }, ref) {
    return /* @__PURE__ */ (0, import_jsx_runtime52.jsx)(
      "div",
      {
        ref,
        className: cn(
          "bg-surface-50 dark:bg-dark-surface-200 flex w-[min(100%,20rem)] flex-shrink-0 flex-col sm:w-80",
          side === "right" ? "border-l" : "border-r",
          className
        ),
        ...props
      }
    );
  }
);
var SidePanelHeader = (0, import_react27.forwardRef)(
  function SidePanelHeader2({ className, ...props }, ref) {
    return /* @__PURE__ */ (0, import_jsx_runtime52.jsx)(
      "div",
      {
        ref,
        className: cn("flex shrink-0 items-center justify-between gap-2 border-b px-2 py-2", className),
        ...props
      }
    );
  }
);
var SidePanelBody = (0, import_react27.forwardRef)(
  function SidePanelBody2({ className, ...props }, ref) {
    return /* @__PURE__ */ (0, import_jsx_runtime52.jsx)(
      "div",
      {
        ref,
        className: cn("flex min-h-0 flex-1 flex-col gap-2 overflow-auto p-2", className),
        ...props
      }
    );
  }
);
var SidePanelRailButton = (0, import_react27.forwardRef)(
  function SidePanelRailButton2({ className, side = "right", type = "button", ...props }, ref) {
    return /* @__PURE__ */ (0, import_jsx_runtime52.jsx)(
      "button",
      {
        ref,
        type,
        className: cn(
          "bg-surface-50 dark:bg-dark-surface-200 text-secondary-600 hover:bg-surface-100 dark:text-dark-secondary-400 dark:hover:bg-dark-surface-300 flex w-9 shrink-0 flex-col items-center justify-center",
          side === "right" ? "border-l" : "border-r",
          className
        ),
        ...props
      }
    );
  }
);

// src/components/Table.tsx
var import_jsx_runtime53 = require("react/jsx-runtime");
function Table({ columns, children, className, ...props }) {
  return /* @__PURE__ */ (0, import_jsx_runtime53.jsx)("div", { className: "border-secondary-200 dark:border-dark-secondary-300 overflow-x-auto rounded-lg border", children: /* @__PURE__ */ (0, import_jsx_runtime53.jsxs)("table", { className: cn("w-full min-w-[600px]", className), ...props, children: [
    /* @__PURE__ */ (0, import_jsx_runtime53.jsx)("thead", { className: "bg-secondary-50 dark:bg-dark-surface-100", children: /* @__PURE__ */ (0, import_jsx_runtime53.jsx)("tr", { children: columns.map((column) => /* @__PURE__ */ (0, import_jsx_runtime53.jsx)(
      "th",
      {
        className: "text-secondary-600 dark:text-dark-secondary-400 px-4 py-3 text-left text-sm font-medium",
        children: column
      },
      column
    )) }) }),
    /* @__PURE__ */ (0, import_jsx_runtime53.jsx)("tbody", { className: "divide-secondary-200 dark:divide-dark-secondary-300 divide-y", children })
  ] }) });
}
function TableRow({ className, ...props }) {
  return /* @__PURE__ */ (0, import_jsx_runtime53.jsx)(
    "tr",
    {
      className: cn(
        "hover:bg-secondary-50 dark:hover:bg-dark-surface-100",
        className
      ),
      ...props
    }
  );
}
function TableCell({ className, ...props }) {
  return /* @__PURE__ */ (0, import_jsx_runtime53.jsx)(
    "td",
    {
      className: cn(
        "text-secondary-800 dark:text-dark-secondary-200 px-4 py-3 text-sm",
        className
      ),
      ...props
    }
  );
}

// src/components/TabTrigger.tsx
var import_react28 = require("react");
var import_jsx_runtime54 = require("react/jsx-runtime");
var TabTrigger = (0, import_react28.forwardRef)(
  ({ active, className, children, ...props }, ref) => /* @__PURE__ */ (0, import_jsx_runtime54.jsx)(
    "button",
    {
      ref,
      role: "tab",
      "aria-selected": active,
      type: "button",
      ...props,
      className: cn(
        "group relative px-5 py-2.5 rounded-none transition-colors",
        "focus:outline-none focus-visible:ring-1 focus-visible:ring-primary/20",
        "hover:bg-white/5",
        active ? "text-primary-500 dark:text-dark-primary-400 after:scale-x-100" : "text-foreground/80 after:scale-x-0 group-hover:after:scale-x-100",
        "after:absolute after:inset-x-3 after:bottom-0 after:h-px after:bg-primary-500/60 dark:after:bg-dark-primary-400/60 after:transition-transform after:origin-left",
        className
      ),
      children
    }
  )
);
TabTrigger.displayName = "TabTrigger";

// src/components/Tabs.tsx
var import_react29 = require("react");
var import_jsx_runtime55 = require("react/jsx-runtime");
function Tabs({
  tabs,
  activeTab,
  onTabChange,
  className
}) {
  const refs = (0, import_react29.useRef)({});
  const onKeyDown = (e) => {
    const idx = tabs.findIndex((t) => t.id === activeTab);
    if (idx === -1) return;
    let nextIdx = idx;
    if (e.key === "ArrowRight") nextIdx = (idx + 1) % tabs.length;
    else if (e.key === "ArrowLeft")
      nextIdx = (idx - 1 + tabs.length) % tabs.length;
    else if (e.key === "Home") nextIdx = 0;
    else if (e.key === "End") nextIdx = tabs.length - 1;
    else return;
    e.preventDefault();
    const nextId = tabs[nextIdx].id;
    onTabChange(nextId);
    refs.current[String(nextId)]?.focus();
  };
  return /* @__PURE__ */ (0, import_jsx_runtime55.jsx)(
    "div",
    {
      role: "tablist",
      "aria-orientation": "horizontal",
      className: cn("flex gap-1", className),
      onKeyDown,
      children: tabs.map((tab) => {
        const isActive = tab.id === activeTab;
        return /* @__PURE__ */ (0, import_jsx_runtime55.jsx)(
          TabTrigger,
          {
            ref: (el) => {
              refs.current[tab.id] = el ?? null;
            },
            active: isActive,
            disabled: tab.disabled,
            id: `tab-${String(tab.id)}`,
            "aria-controls": `panel-${String(tab.id)}`,
            tabIndex: isActive ? 0 : -1,
            onClick: () => onTabChange(tab.id),
            children: tab.label
          },
          String(tab.id)
        );
      })
    }
  );
}

// src/components/TabPanel.tsx
var import_jsx_runtime56 = require("react/jsx-runtime");
function TabPanel({
  tabId,
  activeTab,
  children,
  className,
  lazy = false
}) {
  const isActive = activeTab === tabId;
  if (lazy && !isActive) {
    return null;
  }
  return /* @__PURE__ */ (0, import_jsx_runtime56.jsx)(
    "div",
    {
      id: `panel-${String(tabId)}`,
      role: "tabpanel",
      "aria-labelledby": `tab-${String(tabId)}`,
      hidden: !isActive,
      className: cn(isActive && "flex min-h-0 flex-1 flex-col", className),
      children
    }
  );
}
function TabPanels({
  className,
  children
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime56.jsx)("div", { className: cn("flex min-h-0 min-w-0 flex-1 flex-col", className), children });
}

// src/components/Toast.tsx
var import_lucide_react9 = require("lucide-react");
var import_jsx_runtime57 = require("react/jsx-runtime");
function Toast({ message, variant, className }) {
  return /* @__PURE__ */ (0, import_jsx_runtime57.jsxs)(
    "div",
    {
      className: cn(
        "fixed bottom-4 left-1/2 -translate-x-1/2 rounded-lg p-4 shadow-lg",
        "flex items-center gap-3",
        variant === "success" ? "bg-primary-500 text-surface-50 dark:bg-dark-primary-600" : "bg-error-500 text-surface-50 dark:bg-dark-error-600",
        className
      ),
      children: [
        variant === "success" ? /* @__PURE__ */ (0, import_jsx_runtime57.jsx)(import_lucide_react9.CheckCircle2, { className: "h-5 w-5" }) : /* @__PURE__ */ (0, import_jsx_runtime57.jsx)(import_lucide_react9.XCircle, { className: "h-5 w-5" }),
        /* @__PURE__ */ (0, import_jsx_runtime57.jsx)(Span, { className: "text-sm font-medium", children: message })
      ]
    }
  );
}

// src/components/UserMenu.tsx
var import_lucide_react10 = require("lucide-react");
var import_jsx_runtime58 = require("react/jsx-runtime");
function UserMenu({
  isOpen,
  friendlyName,
  mail,
  logout,
  onToggle,
  className,
  children
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime58.jsx)(
    Dropdown,
    {
      isOpen,
      onToggle,
      trigger: /* @__PURE__ */ (0, import_jsx_runtime58.jsx)(Button, { variant: "ghost", size: "icon", "aria-label": "User Menu", children: /* @__PURE__ */ (0, import_jsx_runtime58.jsx)(import_lucide_react10.User2Icon, { className: "h-6 w-6" }) }),
      className: cn("relative", className),
      contentClassName: "absolute right-0 mt-2 w-48 rounded-lg shadow-lg z-50",
      children: /* @__PURE__ */ (0, import_jsx_runtime58.jsxs)(Section, { children: [
        (friendlyName || mail) && /* @__PURE__ */ (0, import_jsx_runtime58.jsxs)(Span, { children: [
          friendlyName && /* @__PURE__ */ (0, import_jsx_runtime58.jsx)(P, { children: friendlyName }),
          mail && /* @__PURE__ */ (0, import_jsx_runtime58.jsx)(P, { children: mail })
        ] }),
        /* @__PURE__ */ (0, import_jsx_runtime58.jsx)(Span, { children: logout && /* @__PURE__ */ (0, import_jsx_runtime58.jsx)(Button, { onClick: logout, children: "logout" }) }),
        /* @__PURE__ */ (0, import_jsx_runtime58.jsx)(Span, { children: children && children })
      ] })
    }
  );
}

// src/components/Container.tsx
var import_jsx_runtime59 = require("react/jsx-runtime");
function Container({
  title,
  className,
  children,
  padding = "p-6",
  innerPadding = "p-4",
  ...rest
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime59.jsxs)(
    "div",
    {
      className: cn(`container mx-auto space-y-6`, padding, className),
      ...rest,
      children: [
        title && /* @__PURE__ */ (0, import_jsx_runtime59.jsx)(H1, { children: title }),
        /* @__PURE__ */ (0, import_jsx_runtime59.jsx)("div", { className: cn("bg-inherit", innerPadding), children })
      ]
    }
  );
}

// src/components/GridLayout.tsx
var import_jsx_runtime60 = require("react/jsx-runtime");
function GridLayout({
  title,
  description,
  minWidth = "minmax(400px, 1fr)",
  columns = 0,
  responsive,
  variant = "bordered",
  className,
  children,
  ...props
}) {
  let inlineStyle = void 0;
  let responsiveClasses = "";
  if (responsive) {
    const breakpoints = {
      base: "",
      sm: "sm:",
      md: "md:",
      lg: "lg:",
      xl: "xl:"
    };
    const entries = Object.entries({
      base: responsive.base ?? 1,
      ..."sm" in responsive ? { sm: responsive.sm } : {},
      ..."md" in responsive ? { md: responsive.md } : {},
      ..."lg" in responsive ? { lg: responsive.lg } : {},
      ..."xl" in responsive ? { xl: responsive.xl } : {}
    });
    responsiveClasses = entries.map(([bp, value]) => `${breakpoints[bp]}grid-cols-${value}`).join(" ");
  } else {
    inlineStyle = {
      gridTemplateColumns: columns ? `repeat(${columns}, 1fr)` : `repeat(auto-fit, ${minWidth})`
    };
  }
  return /* @__PURE__ */ (0, import_jsx_runtime60.jsx)(
    Section,
    {
      title,
      description,
      variant,
      ...props,
      children: /* @__PURE__ */ (0, import_jsx_runtime60.jsx)(
        "div",
        {
          className: cn(
            "grid gap-4 min-w-0 overflow-x-hidden",
            "[&>*]:min-w-0",
            responsiveClasses,
            className
          ),
          style: inlineStyle,
          children
        }
      )
    }
  );
}

// src/components/TabbedPage.tsx
var import_react30 = require("react");
var import_jsx_runtime61 = require("react/jsx-runtime");
function TabbedPage({
  tabs,
  defaultActiveTab,
  activeTab: controlledActiveTab,
  onTabChange,
  mountActivePanelOnly = false,
  ...props
}) {
  const [activeTab, setActiveTab] = (0, import_react30.useState)(
    controlledActiveTab ?? defaultActiveTab ?? tabs[0]?.id
  );
  const tabRefs = (0, import_react30.useRef)({});
  (0, import_react30.useEffect)(() => {
    if (controlledActiveTab !== void 0 && controlledActiveTab !== activeTab) {
      setActiveTab(controlledActiveTab);
    }
  }, [controlledActiveTab, activeTab]);
  const setAndNotify = (id) => {
    setActiveTab(id);
    onTabChange?.(id);
  };
  const onKeyDown = (e) => {
    const idx = tabs.findIndex((t) => t.id === activeTab);
    if (idx === -1) return;
    let nextIdx = idx;
    if (e.key === "ArrowRight") nextIdx = (idx + 1) % tabs.length;
    else if (e.key === "ArrowLeft")
      nextIdx = (idx - 1 + tabs.length) % tabs.length;
    else if (e.key === "Home") nextIdx = 0;
    else if (e.key === "End") nextIdx = tabs.length - 1;
    else return;
    e.preventDefault();
    const nextId = tabs[nextIdx].id;
    setAndNotify(nextId);
    tabRefs.current[nextId]?.focus();
  };
  return /* @__PURE__ */ (0, import_jsx_runtime61.jsxs)("div", { ...props, className: `space-y-4 ${props.className ?? ""}`, children: [
    /* @__PURE__ */ (0, import_jsx_runtime61.jsx)(
      "div",
      {
        role: "tablist",
        "aria-orientation": "horizontal",
        className: "flex gap-1",
        onKeyDown,
        children: tabs.map((tab) => {
          const isActive = tab.id === activeTab;
          return /* @__PURE__ */ (0, import_jsx_runtime61.jsx)(
            TabTrigger,
            {
              ref: (el) => {
                tabRefs.current[tab.id] = el ?? null;
              },
              active: isActive,
              "aria-controls": `panel-${tab.id}`,
              id: `tab-${tab.id}`,
              tabIndex: isActive ? 0 : -1,
              onClick: () => setAndNotify(tab.id),
              children: tab.label
            },
            tab.id
          );
        })
      }
    ),
    /* @__PURE__ */ (0, import_jsx_runtime61.jsx)("div", { children: tabs.map(({ id, content }) => /* @__PURE__ */ (0, import_jsx_runtime61.jsx)(
      "div",
      {
        role: "tabpanel",
        id: `panel-${id}`,
        "aria-labelledby": `tab-${id}`,
        className: id === activeTab ? "block" : "hidden",
        hidden: id !== activeTab,
        children: mountActivePanelOnly && id !== activeTab ? null : content
      },
      id
    )) })
  ] });
}

// src/components/panels/DetailsPanel.tsx
var import_lucide_react11 = require("lucide-react");
var import_react31 = require("react");
var import_jsx_runtime62 = require("react/jsx-runtime");
var DetailsPanel = ({
  title,
  data,
  fields,
  onClose,
  onSave,
  onDelete,
  isEditing = false,
  onEditToggle,
  onFieldUpdate,
  className
}) => {
  const [editedData, setEditedData] = (0, import_react31.useState)({});
  const [isEditMode, setIsEditMode] = (0, import_react31.useState)(isEditing);
  (0, import_react31.useEffect)(() => {
    setEditedData({ ...data });
  }, [data]);
  const handleSave = () => {
    onSave?.(editedData);
    setIsEditMode(false);
    onEditToggle?.(false);
  };
  const handleCancel = () => {
    setEditedData({ ...data });
    setIsEditMode(false);
    onEditToggle?.(false);
  };
  const updateField = (key, value) => {
    const updates = { [key]: value };
    setEditedData((prev) => ({ ...prev, ...updates }));
    onFieldUpdate?.(updates);
  };
  const renderField = (field) => {
    const rawValue = isEditMode ? editedData[field.key] : data[field.key];
    if (field.render) {
      return field.render(rawValue);
    }
    const value = typeof rawValue === "string" ? rawValue : String(rawValue ?? "");
    switch (field.type) {
      case "badge":
        return /* @__PURE__ */ (0, import_jsx_runtime62.jsx)(Badge, { children: value });
      case "select":
        return isEditMode ? /* @__PURE__ */ (0, import_jsx_runtime62.jsx)(
          Select,
          {
            value,
            onChange: (e) => updateField(field.key, e.target.value),
            options: field.options || [],
            className: "bg-surface-50 dark:bg-dark-surface-50 border-surface-300 dark:border-dark-surface-300 focus:border-primary-500 dark:focus:border-dark-primary-500 focus:ring-primary-500 dark:focus:ring-dark-primary-500"
          }
        ) : /* @__PURE__ */ (0, import_jsx_runtime62.jsx)("div", { className: "text-sm text-text dark:text-dark-text", children: value });
      case "textarea":
        return isEditMode ? /* @__PURE__ */ (0, import_jsx_runtime62.jsx)(
          Textarea,
          {
            value,
            onChange: (e) => updateField(field.key, e.target.value),
            className: "min-h-[80px] bg-surface-50 dark:bg-dark-surface-50 border-surface-300 dark:border-dark-surface-300 focus:border-primary-500 dark:focus:border-dark-primary-500 focus:ring-primary-500 dark:focus:ring-dark-primary-500"
          }
        ) : /* @__PURE__ */ (0, import_jsx_runtime62.jsx)("div", { className: "bg-surface-100 dark:bg-dark-surface-100 rounded p-2 font-mono text-sm text-text dark:text-dark-text", children: value });
      default:
        return isEditMode ? /* @__PURE__ */ (0, import_jsx_runtime62.jsx)(
          Input,
          {
            value,
            onChange: (e) => updateField(field.key, e.target.value),
            className: "bg-surface-50 dark:bg-dark-surface-50 border-surface-300 dark:border-dark-surface-300 focus:border-primary-500 dark:focus:border-dark-primary-500 focus:ring-primary-500 dark:focus:ring-dark-primary-500"
          }
        ) : /* @__PURE__ */ (0, import_jsx_runtime62.jsx)("div", { className: "text-sm text-text dark:text-dark-text", children: value });
    }
  };
  return /* @__PURE__ */ (0, import_jsx_runtime62.jsxs)(
    "div",
    {
      className: `bg-surface-50 dark:bg-dark-surface-50 flex h-full w-96 flex-col border-l border-surface-300 dark:border-dark-surface-300 shadow-xl ${className}`,
      children: [
        /* @__PURE__ */ (0, import_jsx_runtime62.jsxs)("div", { className: "flex items-center justify-between border-b border-surface-300 dark:border-dark-surface-300 p-4", children: [
          /* @__PURE__ */ (0, import_jsx_runtime62.jsxs)("div", { children: [
            /* @__PURE__ */ (0, import_jsx_runtime62.jsx)("h4", { className: "text-lg font-semibold text-text dark:text-dark-text", children: title }),
            !isEditMode && /* @__PURE__ */ (0, import_jsx_runtime62.jsx)(Badge, { className: "mt-1", children: "View Mode" })
          ] }),
          /* @__PURE__ */ (0, import_jsx_runtime62.jsxs)("div", { className: "flex gap-1", children: [
            !isEditMode && onSave && /* @__PURE__ */ (0, import_jsx_runtime62.jsx)(
              Button,
              {
                size: "sm",
                variant: "secondary",
                onClick: () => {
                  setIsEditMode(true);
                  onEditToggle?.(true);
                },
                children: "Edit"
              }
            ),
            onDelete && /* @__PURE__ */ (0, import_jsx_runtime62.jsx)(Button, { size: "sm", variant: "ghost", onClick: onDelete, children: /* @__PURE__ */ (0, import_jsx_runtime62.jsx)(import_lucide_react11.Trash2, { className: "h-4 w-4 text-error-500 dark:text-dark-error-500" }) }),
            /* @__PURE__ */ (0, import_jsx_runtime62.jsx)(Button, { size: "icon", variant: "ghost", onClick: onClose, children: /* @__PURE__ */ (0, import_jsx_runtime62.jsx)(import_lucide_react11.X, { className: "h-4 w-4 text-text dark:text-dark-text" }) })
          ] })
        ] }),
        /* @__PURE__ */ (0, import_jsx_runtime62.jsx)("div", { className: "flex-1 space-y-4 overflow-y-auto p-4", children: fields.map((field) => /* @__PURE__ */ (0, import_jsx_runtime62.jsxs)(Panel, { variant: "surface", children: [
          /* @__PURE__ */ (0, import_jsx_runtime62.jsx)(Label, { className: "text-text dark:text-dark-text", children: field.label }),
          /* @__PURE__ */ (0, import_jsx_runtime62.jsx)("div", { className: "mt-2", children: renderField(field) })
        ] }, field.key)) }),
        isEditMode && /* @__PURE__ */ (0, import_jsx_runtime62.jsx)("div", { className: "border-t border-surface-300 dark:border-dark-surface-300 p-4", children: /* @__PURE__ */ (0, import_jsx_runtime62.jsxs)("div", { className: "flex gap-2", children: [
          /* @__PURE__ */ (0, import_jsx_runtime62.jsx)(
            Button,
            {
              variant: "secondary",
              onClick: handleCancel,
              className: "flex-1",
              children: "Cancel"
            }
          ),
          /* @__PURE__ */ (0, import_jsx_runtime62.jsxs)(Button, { variant: "primary", onClick: handleSave, className: "flex-1", children: [
            /* @__PURE__ */ (0, import_jsx_runtime62.jsx)(import_lucide_react11.Save, { className: "mr-2 h-4 w-4" }),
            "Save Changes"
          ] })
        ] }) })
      ]
    }
  );
};

// src/components/forms/TabbedForm.tsx
var import_react32 = require("react");
var import_jsx_runtime63 = require("react/jsx-runtime");
var TabbedForm = ({
  title,
  description,
  tabs,
  onSave,
  onCancel,
  onDelete,
  className
}) => {
  const [activeTab, setActiveTab] = (0, import_react32.useState)(tabs[0]?.id);
  const tabRefs = (0, import_react32.useRef)({});
  const handleTabChange = (0, import_react32.useCallback)((tabId) => {
    setActiveTab(tabId);
  }, []);
  const activeTabContent = tabs.find((t) => t.id === activeTab)?.content;
  const onKeyDown = (e) => {
    const idx = tabs.findIndex((t) => t.id === activeTab);
    if (idx === -1) return;
    let nextIdx = idx;
    if (e.key === "ArrowRight") nextIdx = (idx + 1) % tabs.length;
    else if (e.key === "ArrowLeft")
      nextIdx = (idx - 1 + tabs.length) % tabs.length;
    else if (e.key === "Home") nextIdx = 0;
    else if (e.key === "End") nextIdx = tabs.length - 1;
    else return;
    e.preventDefault();
    const nextId = tabs[nextIdx].id;
    setActiveTab(nextId);
    tabRefs.current[nextId]?.focus();
  };
  return /* @__PURE__ */ (0, import_jsx_runtime63.jsxs)("div", { className: `flex h-full flex-col ${className ?? ""}`, children: [
    /* @__PURE__ */ (0, import_jsx_runtime63.jsx)(Section, { title, description, className: "shrink-0", children: /* @__PURE__ */ (0, import_jsx_runtime63.jsx)(
      "div",
      {
        role: "tablist",
        "aria-orientation": "horizontal",
        className: "flex gap-1",
        onKeyDown,
        children: tabs.map((tab) => {
          const isActive = tab.id === activeTab;
          return /* @__PURE__ */ (0, import_jsx_runtime63.jsx)(
            TabTrigger,
            {
              ref: (el) => {
                tabRefs.current[tab.id] = el ?? null;
              },
              active: isActive,
              "aria-controls": `panel-${tab.id}`,
              id: `tab-${tab.id}`,
              tabIndex: isActive ? 0 : -1,
              onClick: () => handleTabChange(tab.id),
              disabled: tab.disabled,
              children: tab.label
            },
            tab.id
          );
        })
      }
    ) }),
    /* @__PURE__ */ (0, import_jsx_runtime63.jsx)("div", { className: "flex-1 overflow-y-auto", children: activeTabContent && /* @__PURE__ */ (0, import_jsx_runtime63.jsx)(
      "div",
      {
        role: "tabpanel",
        id: `panel-${activeTab}`,
        "aria-labelledby": `tab-${activeTab}`,
        className: "block",
        children: activeTabContent
      }
    ) }),
    /* @__PURE__ */ (0, import_jsx_runtime63.jsx)("div", { className: "mt-6 shrink-0 border-t border-surface-300 dark:border-dark-surface-400 pt-4", children: /* @__PURE__ */ (0, import_jsx_runtime63.jsxs)("div", { className: "flex items-center justify-between", children: [
      /* @__PURE__ */ (0, import_jsx_runtime63.jsx)("div", { children: onDelete && /* @__PURE__ */ (0, import_jsx_runtime63.jsx)(Button, { variant: "secondary", onClick: onDelete, children: "Delete" }) }),
      /* @__PURE__ */ (0, import_jsx_runtime63.jsxs)("div", { className: "flex gap-2", children: [
        /* @__PURE__ */ (0, import_jsx_runtime63.jsx)(Button, { variant: "secondary", onClick: onCancel, children: "Cancel" }),
        /* @__PURE__ */ (0, import_jsx_runtime63.jsx)(Button, { variant: "primary", onClick: onSave, children: "Save" })
      ] })
    ] }) })
  ] });
};

// src/components/visualization/LayoutControls.tsx
var import_lucide_react12 = require("lucide-react");
var import_jsx_runtime64 = require("react/jsx-runtime");
var LayoutControls = ({
  direction,
  onChangeDirection
}) => {
  return /* @__PURE__ */ (0, import_jsx_runtime64.jsxs)("div", { className: "flex gap-1 rounded-md border border-surface-300 dark:border-dark-surface-300 bg-surface-50 dark:bg-dark-surface-50 p-1", children: [
    /* @__PURE__ */ (0, import_jsx_runtime64.jsx)(
      Button,
      {
        size: "icon",
        variant: direction === "horizontal" ? "primary" : "secondary",
        onClick: () => onChangeDirection("horizontal"),
        "aria-label": "Horizontal layout",
        className: `${direction === "horizontal" ? "bg-primary-500 dark:bg-dark-primary-500 text-white hover:bg-primary-600 dark:hover:bg-dark-primary-600" : "bg-surface-100 dark:bg-dark-surface-100 text-text dark:text-dark-text hover:bg-surface-200 dark:hover:bg-dark-surface-200"}`,
        children: /* @__PURE__ */ (0, import_jsx_runtime64.jsx)(import_lucide_react12.LayoutGrid, { className: "h-4 w-4" })
      }
    ),
    /* @__PURE__ */ (0, import_jsx_runtime64.jsx)(
      Button,
      {
        size: "icon",
        variant: direction === "vertical" ? "primary" : "secondary",
        onClick: () => onChangeDirection("vertical"),
        "aria-label": "Vertical layout",
        className: `${direction === "vertical" ? "bg-primary-500 dark:bg-dark-primary-500 text-white hover:bg-primary-600 dark:hover:bg-dark-primary-600" : "bg-surface-100 dark:bg-dark-surface-100 text-text dark:text-dark-text hover:bg-surface-200 dark:hover:bg-dark-surface-200"}`,
        children: /* @__PURE__ */ (0, import_jsx_runtime64.jsx)(import_lucide_react12.LayoutList, { className: "h-4 w-4" })
      }
    )
  ] });
};

// src/components/visualization/AddNodeButton.tsx
var import_lucide_react13 = require("lucide-react");
var import_jsx_runtime65 = require("react/jsx-runtime");
var AddNodeButton = ({
  x,
  y,
  onClick,
  className
}) => {
  return /* @__PURE__ */ (0, import_jsx_runtime65.jsxs)(
    "g",
    {
      transform: `translate(${x - 12}, ${y - 12})`,
      className: `cursor-pointer ${className}`,
      children: [
        /* @__PURE__ */ (0, import_jsx_runtime65.jsx)(
          "circle",
          {
            cx: "12",
            cy: "12",
            r: "12",
            className: "fill-primary-500 dark:fill-dark-primary-500 hover:fill-primary-600 dark:hover:fill-dark-primary-600 transition-colors duration-200"
          }
        ),
        /* @__PURE__ */ (0, import_jsx_runtime65.jsx)("foreignObject", { width: "24", height: "24", x: "0", y: "0", children: /* @__PURE__ */ (0, import_jsx_runtime65.jsx)("div", { className: "flex h-6 w-6 items-center justify-center", children: /* @__PURE__ */ (0, import_jsx_runtime65.jsx)(
          Button,
          {
            size: "icon",
            variant: "ghost",
            className: "h-6 w-6 text-text-inverted dark:text-dark-text-inverted hover:bg-primary-600 dark:hover:bg-dark-primary-600 hover:text-text-inverted dark:hover:text-dark-text-inverted",
            onClick,
            children: /* @__PURE__ */ (0, import_jsx_runtime65.jsx)(import_lucide_react13.Plus, { className: "h-3 w-3" })
          }
        ) }) })
      ]
    }
  );
};

// src/components/visualization/WorkflowNode.tsx
var import_lucide_react14 = require("lucide-react");
var import_jsx_runtime66 = require("react/jsx-runtime");
var WorkflowNode = ({
  id,
  label,
  type,
  description,
  metadata,
  position,
  isSelected = false,
  onClick,
  className
}) => {
  const { x, y, width, height } = position;
  const handleClick = () => {
    onClick?.(id);
  };
  const statusStrokes = {
    default: "stroke-surface-300 dark:stroke-dark-surface-600",
    success: "stroke-success-500 dark:stroke-dark-success-500",
    error: "stroke-error-500 dark:stroke-dark-error-500",
    warning: "stroke-warning-500 dark:stroke-dark-warning-500"
  };
  const status = metadata?.status || "default";
  return /* @__PURE__ */ (0, import_jsx_runtime66.jsxs)(
    "g",
    {
      transform: `translate(${x}, ${y})`,
      className: cn("cursor-pointer", className),
      onClick: handleClick,
      children: [
        /* @__PURE__ */ (0, import_jsx_runtime66.jsx)(
          "rect",
          {
            width,
            height,
            rx: "12",
            className: cn(
              "fill-surface-50 dark:fill-dark-surface-50 stroke-2 transition-all duration-300 ease-in-out",
              "shadow-md hover:shadow-lg",
              statusStrokes[status],
              isSelected ? "stroke-accent-500 dark:stroke-dark-accent-400" : ""
            )
          }
        ),
        /* @__PURE__ */ (0, import_jsx_runtime66.jsx)("foreignObject", { width, height, children: /* @__PURE__ */ (0, import_jsx_runtime66.jsxs)("div", { className: "flex h-full flex-col p-3", children: [
          /* @__PURE__ */ (0, import_jsx_runtime66.jsx)("div", { className: "flex items-start justify-between", children: /* @__PURE__ */ (0, import_jsx_runtime66.jsxs)("div", { className: "grow overflow-hidden", children: [
            /* @__PURE__ */ (0, import_jsx_runtime66.jsx)(
              "div",
              {
                className: "truncate font-medium text-text dark:text-dark-text",
                title: label,
                children: label
              }
            ),
            /* @__PURE__ */ (0, import_jsx_runtime66.jsx)("div", { className: "truncate text-sm text-text-muted dark:text-dark-text-muted", children: type })
          ] }) }),
          description && /* @__PURE__ */ (0, import_jsx_runtime66.jsx)("div", { className: "mt-2 line-clamp-2 grow text-sm text-text-muted dark:text-dark-text-muted", children: description }),
          metadata?.branches !== void 0 && /* @__PURE__ */ (0, import_jsx_runtime66.jsxs)("div", { className: "mt-2 flex items-center justify-end text-xs text-text-muted dark:text-dark-text-muted", children: [
            /* @__PURE__ */ (0, import_jsx_runtime66.jsx)(import_lucide_react14.GitBranch, { className: "mr-1 h-3 w-3" }),
            /* @__PURE__ */ (0, import_jsx_runtime66.jsxs)("span", { children: [
              metadata.branches,
              " branch",
              metadata.branches !== 1 && "es"
            ] })
          ] })
        ] }) })
      ]
    }
  );
};

// src/components/visualization/WorkflowEdge.tsx
var import_react33 = require("react");

// src/components/visualization/utils.ts
var dagre = __toESM(require("@dagrejs/dagre"));
var NODE_WIDTH = 250;
var NODE_HEIGHT = 120;
var HORIZONTAL_SPACING = 85;
var VERTICAL_SPACING = 100;
var ADD_BTN_RADIUS = 12;
var MIN_BTN_SEPARATION = ADD_BTN_RADIUS * 2 + 4;
var NUDGE = 26;
var getConnectorPath = (source, target, direction) => {
  if (direction === "vertical") {
    const startX = source.x + source.width / 2;
    const startY = source.y + source.height;
    const endX = target.x + target.width / 2;
    const endY = target.y;
    const midY = startY + (endY - startY) / 2;
    return `M${startX},${startY} C${startX},${midY} ${endX},${midY} ${endX},${endY}`;
  } else {
    const startX = source.x + source.width;
    const startY = source.y + source.height / 2;
    const endX = target.x;
    const endY = target.y + target.height / 2;
    const midX = startX + (endX - startX) / 2;
    return `M${startX},${startY} C${midX},${startY} ${midX},${endY} ${endX},${endY}`;
  }
};
var calculateLayout = (nodes, edges, direction) => {
  if (nodes.length === 0) {
    return { nodePositions: {}, edges: [], addButtons: [] };
  }
  const graph = new dagre.graphlib.Graph();
  graph.setGraph({
    rankdir: direction === "horizontal" ? "LR" : "TB",
    nodesep: HORIZONTAL_SPACING,
    ranksep: VERTICAL_SPACING,
    marginx: 25,
    marginy: 25
  });
  graph.setDefaultEdgeLabel(() => ({}));
  nodes.forEach((node) => {
    graph.setNode(node.id, {
      width: NODE_WIDTH,
      height: NODE_HEIGHT,
      label: node.id
    });
  });
  edges.forEach((edge) => {
    graph.setEdge(edge.from, edge.to);
  });
  dagre.layout(graph);
  const nodePositions = {};
  graph.nodes().forEach((id) => {
    const node = graph.node(id);
    if (node) {
      nodePositions[id] = {
        id,
        x: node.x - node.width / 2,
        y: node.y - node.height / 2,
        width: node.width,
        height: node.height
      };
    }
  });
  const addButtons = [];
  const isTooClose = (x, y) => addButtons.some((b) => {
    const dx = b.x - x;
    const dy = b.y - y;
    return Math.hypot(dx, dy) < MIN_BTN_SEPARATION;
  });
  const resolveCollision = (x, y) => {
    if (!isTooClose(x, y)) return { x, y };
    const candidates = direction === "vertical" ? [
      { x: x + NUDGE, y },
      { x: x - NUDGE, y },
      { x: x + NUDGE * 2, y },
      { x: x - NUDGE * 2, y }
    ] : [
      { x, y: y + NUDGE },
      { x, y: y - NUDGE },
      { x, y: y + NUDGE * 2 },
      { x, y: y - NUDGE * 2 }
    ];
    for (const c of candidates) {
      if (!isTooClose(c.x, c.y)) return c;
    }
    return { x, y };
  };
  edges.forEach((edge) => {
    const from = nodePositions[edge.from];
    const to = nodePositions[edge.to];
    if (!from || !to) return;
    let x;
    let y;
    if (direction === "vertical") {
      x = (from.x + from.width / 2 + to.x + to.width / 2) / 2;
      y = from.y + from.height + (to.y - (from.y + from.height)) / 2;
    } else {
      x = from.x + from.width + (to.x - (from.x + from.width)) / 2;
      y = (from.y + from.height / 2 + to.y + to.height / 2) / 2;
    }
    const pos = resolveCollision(x, y);
    addButtons.push({
      x: pos.x,
      y: pos.y,
      fromNodeId: edge.from,
      toNodeId: edge.to
    });
  });
  nodes.forEach((node) => {
    const nodePos = nodePositions[node.id];
    if (!nodePos) return;
    let x;
    let y;
    if (direction === "vertical") {
      x = nodePos.x + nodePos.width / 2;
      y = nodePos.y + nodePos.height + 40;
    } else {
      x = nodePos.x + nodePos.width + 40;
      y = nodePos.y + nodePos.height / 2;
    }
    const pos = resolveCollision(x, y);
    addButtons.push({
      x: pos.x,
      y: pos.y,
      fromNodeId: node.id
    });
  });
  return { nodePositions, edges, addButtons };
};

// src/components/visualization/WorkflowEdge.tsx
var import_jsx_runtime67 = require("react/jsx-runtime");
var ONE_LINE_HEIGHT = 22;
var TWO_LINE_HEIGHT = 34;
var LABEL_PAD_X = 10;
var LABEL_MIN_W = 44;
var LABEL_MAX_W = 200;
var BUTTON_RADIUS = 12;
var COLLISION_MARGIN = 10;
var NODE_PADDING = 8;
var measureTextWidth = (text, approxCharPx = 7) => Math.max(
  LABEL_MIN_W,
  Math.min(LABEL_MAX_W, text.length * approxCharPx + LABEL_PAD_X * 2)
);
var circleIntersectsRect = (cx, cy, r, rect, pad = 0) => {
  const pr = r + pad;
  const clampedX = Math.max(rect.left, Math.min(cx, rect.right));
  const clampedY = Math.max(rect.top, Math.min(cy, rect.bottom));
  const dx = cx - clampedX;
  const dy = cy - clampedY;
  return dx * dx + dy * dy <= pr * pr;
};
var rectsOverlap = (a, b, pad = 0) => {
  return !(a.right < b.left - pad || a.left > b.right + pad || a.bottom < b.top - pad || a.top > b.bottom + pad);
};
var STRATEGY_STYLE = {
  override: { fill: "#525252", stroke: "#171717", short: "OVR" },
  merge_chat_histories: { fill: "#737373", stroke: "#262626", short: "MERGE" },
  append_string_to_chat_history: {
    fill: "#a3a3a3",
    stroke: "#404040",
    short: "APPEND"
  },
  default: { fill: "#64748b", stroke: "#334155", short: "DEFAULT" }
};
var getStyleForStrategy = (s) => STRATEGY_STYLE[(s || "default").toLowerCase()] ?? STRATEGY_STYLE.default;
var WorkflowEdge = ({
  source,
  target,
  label,
  direction,
  isHighlighted = false,
  isError = false,
  className,
  addButtonPositions = [],
  hasCompose = false,
  composeStrategy,
  onComposeClick
}) => {
  if (!source || !target) return null;
  const getEdgeStrokeClass = () => {
    if (isError) return "stroke-error-500 dark:stroke-dark-error-500";
    if (isHighlighted) return "stroke-accent-500 dark:stroke-dark-accent-400";
    return "stroke-primary-500 dark:stroke-dark-primary-500";
  };
  const path = getConnectorPath(source, target, direction);
  const strokeClass = getEdgeStrokeClass();
  const fillClass = isError ? "fill-error-500 dark:fill-dark-error-500" : isHighlighted ? "fill-accent-500 dark:fill-dark-accent-400" : "fill-primary-500 dark:fill-dark-primary-500";
  const strokeWidth = isHighlighted ? 2.5 : 1.5;
  const centerX = (source.x + source.width / 2 + target.x + target.width / 2) / 2;
  const centerY = (source.y + source.height / 2 + target.y + target.height / 2) / 2;
  const sx = source.x + source.width / 2;
  const sy = source.y + source.height / 2;
  const tx = target.x + target.width / 2;
  const ty = target.y + target.height / 2;
  const dx = tx - sx;
  const dy = ty - sy;
  const horizontalDominant = Math.abs(dx) >= Math.abs(dy);
  const srcRect = (0, import_react33.useMemo)(
    () => ({
      left: source.x,
      right: source.x + source.width,
      top: source.y,
      bottom: source.y + source.height
    }),
    [source.x, source.y, source.width, source.height]
  );
  const tgtRect = (0, import_react33.useMemo)(
    () => ({
      left: target.x,
      right: target.x + target.width,
      top: target.y,
      bottom: target.y + target.height
    }),
    [target.x, target.y, target.width, target.height]
  );
  const {
    fill: chipFill,
    stroke: chipStroke,
    short: shortStrat
  } = getStyleForStrategy(composeStrategy);
  const line1 = (label || "default").trim();
  const line2 = hasCompose ? shortStrat : "";
  const chipWidth = (0, import_react33.useMemo)(
    () => Math.max(
      measureTextWidth(line1),
      hasCompose ? measureTextWidth(line2, 6.5) : 0
    ),
    [line1, line2, hasCompose]
  );
  const chipHeight = hasCompose ? TWO_LINE_HEIGHT : ONE_LINE_HEIGHT;
  const halfW = chipWidth / 2;
  const halfH = chipHeight / 2;
  const normalOffset = BUTTON_RADIUS + halfH + 10;
  const alongOffset = BUTTON_RADIUS + halfW + 10;
  const diagOffsetX = halfW + BUTTON_RADIUS + 12;
  const diagOffsetY = halfH + BUTTON_RADIUS + 12;
  const candidates = (0, import_react33.useMemo)(() => {
    if (horizontalDominant) {
      const primary = [
        { x: centerX, y: centerY - normalOffset },
        { x: centerX, y: centerY + normalOffset }
      ];
      const diagonals = [
        { x: centerX - diagOffsetX, y: centerY - diagOffsetY },
        // TL
        { x: centerX + diagOffsetX, y: centerY - diagOffsetY },
        // TR
        { x: centerX - diagOffsetX, y: centerY + diagOffsetY },
        // BL
        { x: centerX + diagOffsetX, y: centerY + diagOffsetY }
        // BR
      ];
      const lateral = dx >= 0 ? [
        { x: centerX - alongOffset, y: centerY },
        { x: centerX + alongOffset, y: centerY }
      ] : [
        { x: centerX + alongOffset, y: centerY },
        { x: centerX - alongOffset, y: centerY }
      ];
      return [...primary, ...diagonals, ...lateral];
    } else {
      const lateral = dx >= 0 ? [
        { x: centerX - alongOffset, y: centerY },
        { x: centerX + alongOffset, y: centerY }
      ] : [
        { x: centerX + alongOffset, y: centerY },
        { x: centerX - alongOffset, y: centerY }
      ];
      const diagonals = dy >= 0 ? [
        { x: centerX - diagOffsetX, y: centerY - diagOffsetY },
        { x: centerX + diagOffsetX, y: centerY - diagOffsetY },
        { x: centerX - diagOffsetX, y: centerY + diagOffsetY },
        { x: centerX + diagOffsetX, y: centerY + diagOffsetY }
      ] : [
        { x: centerX - diagOffsetX, y: centerY + diagOffsetY },
        { x: centerX + diagOffsetX, y: centerY + diagOffsetY },
        { x: centerX - diagOffsetX, y: centerY - diagOffsetY },
        { x: centerX + diagOffsetX, y: centerY - diagOffsetY }
      ];
      const vertical = [
        { x: centerX, y: centerY - normalOffset },
        { x: centerX, y: centerY + normalOffset }
      ];
      return [...lateral, ...diagonals, ...vertical];
    }
  }, [
    horizontalDominant,
    dx,
    dy,
    centerX,
    centerY,
    normalOffset,
    alongOffset,
    diagOffsetX,
    diagOffsetY
  ]);
  const candidateIsSafe = (cx, cy) => {
    const rect = {
      left: cx - halfW,
      right: cx + halfW,
      top: cy - halfH,
      bottom: cy + halfH
    };
    const hitsButton = addButtonPositions.some(
      (btn) => circleIntersectsRect(btn.x, btn.y, BUTTON_RADIUS, rect, COLLISION_MARGIN)
    );
    if (hitsButton) return false;
    const hitsNodes = rectsOverlap(rect, srcRect, NODE_PADDING) || rectsOverlap(rect, tgtRect, NODE_PADDING);
    if (hitsNodes) return false;
    return true;
  };
  const labelCenter = (0, import_react33.useMemo)(() => {
    for (const c of candidates) {
      if (candidateIsSafe(c.x, c.y)) return c;
    }
    return candidates[0];
  }, [
    candidates,
    addButtonPositions,
    halfW,
    halfH,
    srcRect.left,
    srcRect.right,
    srcRect.top,
    srcRect.bottom,
    tgtRect.left,
    tgtRect.right,
    tgtRect.top,
    tgtRect.bottom
  ]);
  const handleChipClick = (e) => {
    e.stopPropagation();
    onComposeClick?.();
  };
  return /* @__PURE__ */ (0, import_jsx_runtime67.jsxs)("g", { className: cn(className), children: [
    /* @__PURE__ */ (0, import_jsx_runtime67.jsx)("defs", { children: /* @__PURE__ */ (0, import_jsx_runtime67.jsx)(
      "marker",
      {
        id: "arrowhead",
        viewBox: "0 0 10 10",
        refX: "8",
        refY: "5",
        markerWidth: "6",
        markerHeight: "6",
        orient: "auto-start-reverse",
        children: /* @__PURE__ */ (0, import_jsx_runtime67.jsx)(
          "path",
          {
            d: "M 0 0 L 10 5 L 0 10 z",
            className: `${strokeClass} ${fillClass}`
          }
        )
      }
    ) }),
    /* @__PURE__ */ (0, import_jsx_runtime67.jsx)(
      "path",
      {
        d: path,
        fill: "none",
        className: cn("transition-all duration-300", strokeClass),
        strokeWidth,
        markerEnd: "url(#arrowhead)"
      }
    ),
    /* @__PURE__ */ (0, import_jsx_runtime67.jsxs)(
      "g",
      {
        transform: `translate(${labelCenter.x}, ${labelCenter.y})`,
        className: "cursor-pointer select-none",
        onClick: handleChipClick,
        pointerEvents: "all",
        role: "button",
        "aria-label": `Transition: ${line1}${hasCompose ? `. Strategy ${composeStrategy ?? "default"}` : ""}`,
        children: [
          /* @__PURE__ */ (0, import_jsx_runtime67.jsx)(
            "rect",
            {
              x: -halfW,
              y: -halfH,
              width: chipWidth,
              height: chipHeight,
              rx: "12",
              strokeWidth: 1.25,
              fill: chipFill,
              stroke: chipStroke,
              className: "shadow-sm"
            }
          ),
          hasCompose ? /* @__PURE__ */ (0, import_jsx_runtime67.jsxs)(import_jsx_runtime67.Fragment, { children: [
            /* @__PURE__ */ (0, import_jsx_runtime67.jsx)(
              "text",
              {
                x: 0,
                y: -3,
                textAnchor: "middle",
                dominantBaseline: "central",
                fontSize: "11",
                fontWeight: 700,
                fill: "white",
                pointerEvents: "none",
                children: line1
              }
            ),
            /* @__PURE__ */ (0, import_jsx_runtime67.jsx)(
              "text",
              {
                x: 0,
                y: 10,
                textAnchor: "middle",
                dominantBaseline: "central",
                fontSize: "10",
                fontWeight: 600,
                fill: "white",
                opacity: 0.95,
                pointerEvents: "none",
                children: line2
              }
            )
          ] }) : /* @__PURE__ */ (0, import_jsx_runtime67.jsx)(
            "text",
            {
              x: 0,
              y: 1,
              textAnchor: "middle",
              dominantBaseline: "middle",
              fontSize: "11",
              fontWeight: 700,
              fill: "white",
              pointerEvents: "none",
              children: line1
            }
          ),
          /* @__PURE__ */ (0, import_jsx_runtime67.jsx)("title", { children: hasCompose ? `Compose strategy: ${composeStrategy ?? "default"}` : "Click to add compose" })
        ]
      }
    )
  ] });
};

// src/components/visualization/WorkflowVisualizer.tsx
var import_react34 = require("react");
var import_jsx_runtime68 = require("react/jsx-runtime");
function cn2(...xs) {
  return xs.filter(Boolean).join(" ");
}
var WorkflowVisualizer = ({
  debug = false,
  height,
  contentBounds,
  initialZoom = 1,
  className,
  children,
  scrollOnOverflow = false
}) => {
  const containerRef = (0, import_react34.useRef)(null);
  const svgRef = (0, import_react34.useRef)(null);
  const [containerPx, setContainerPx] = (0, import_react34.useState)({ w: 0, h: 0 });
  const [zoom, setZoom] = (0, import_react34.useState)(initialZoom);
  const [viewBox, setViewBox] = (0, import_react34.useState)(() => ({
    x: 0,
    y: 0,
    width: 100,
    height: 100
  }));
  const userAdjustedRef = (0, import_react34.useRef)(false);
  (0, import_react34.useLayoutEffect)(() => {
    const el = containerRef.current;
    if (!el) return;
    const measure = () => {
      const r = el.getBoundingClientRect();
      setContainerPx({
        w: Math.max(r.width | 0, 0),
        h: Math.max(r.height | 0, 0)
      });
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const PAD = 35;
  const fitBox = {
    x: contentBounds.x - PAD,
    y: contentBounds.y - PAD,
    width: Math.max(1, contentBounds.width + PAD * 2),
    height: Math.max(1, contentBounds.height + PAD * 2)
  };
  (0, import_react34.useEffect)(() => {
    if (scrollOnOverflow) return;
    if (userAdjustedRef.current) return;
    setViewBox(fitBox);
  }, [fitBox.x, fitBox.y, fitBox.width, fitBox.height, scrollOnOverflow]);
  const gridId = (0, import_react34.useMemo)(
    () => `grid-${Math.random().toString(36).slice(2)}`,
    []
  );
  const DebugViewBoxRect = debug ? /* @__PURE__ */ (0, import_jsx_runtime68.jsx)(
    "rect",
    {
      x: viewBox.x,
      y: viewBox.y,
      width: viewBox.width,
      height: viewBox.height,
      fill: "none",
      stroke: "blue",
      strokeWidth: 1,
      pointerEvents: "none"
    }
  ) : null;
  const DebugContentRect = debug ? /* @__PURE__ */ (0, import_jsx_runtime68.jsx)(
    "rect",
    {
      x: contentBounds.x,
      y: contentBounds.y,
      width: contentBounds.width,
      height: contentBounds.height,
      fill: "none",
      stroke: "orange",
      strokeDasharray: "4 3",
      strokeWidth: 1,
      pointerEvents: "none"
    }
  ) : null;
  const containerRing = debug ? "ring-2 ring-fuchsia-500" : "";
  const zoomIn = () => {
    userAdjustedRef.current = true;
    setZoom((z) => Math.min(z * 1.2, 8));
  };
  const zoomOut = () => {
    userAdjustedRef.current = true;
    setZoom((z) => Math.max(z / 1.2, 0.05));
  };
  const resetZoom = () => {
    userAdjustedRef.current = false;
    setZoom(1);
    if (!scrollOnOverflow) setViewBox(fitBox);
  };
  const zoomForRender = scrollOnOverflow ? 1 : zoom;
  const sceneW = scrollOnOverflow ? Math.max(1, fitBox.width * zoom) : void 0;
  const sceneH = scrollOnOverflow ? Math.max(1, fitBox.height * zoom) : void 0;
  return /* @__PURE__ */ (0, import_jsx_runtime68.jsxs)("div", { className: "relative flex h-full min-h-0 flex-col", children: [
    /* @__PURE__ */ (0, import_jsx_runtime68.jsxs)("div", { className: "z-10 flex items-center justify-between border-b border-surface-300 dark:border-dark-surface-300 py-2 px-3", children: [
      /* @__PURE__ */ (0, import_jsx_runtime68.jsxs)("h3", { className: "flex items-center gap-2 text-lg font-semibold text-text dark:text-dark-text", children: [
        /* @__PURE__ */ (0, import_jsx_runtime68.jsxs)(
          "svg",
          {
            xmlns: "http://www.w3.org/2000/svg",
            width: "18",
            height: "18",
            viewBox: "0 0 24 24",
            fill: "none",
            stroke: "currentColor",
            strokeWidth: "2",
            strokeLinecap: "round",
            strokeLinejoin: "round",
            className: "lucide lucide-workflow h-5 w-5",
            children: [
              /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("rect", { width: "8", height: "8", x: "3", y: "3", rx: "2" }),
              /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("path", { d: "M7 11v4a2 2 0 0 0 2 2h4" }),
              /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("rect", { width: "8", height: "8", x: "13", y: "13", rx: "2" })
            ]
          }
        ),
        "Workflow"
      ] }),
      /* @__PURE__ */ (0, import_jsx_runtime68.jsxs)("div", { className: "flex items-center gap-2", children: [
        /* @__PURE__ */ (0, import_jsx_runtime68.jsx)(
          "button",
          {
            onClick: zoomOut,
            className: "inline-flex items-center rounded-lg p-2.5 hover:bg-surface-200 dark:hover:bg-dark-surface-200",
            "aria-label": "Zoom out",
            children: /* @__PURE__ */ (0, import_jsx_runtime68.jsxs)(
              "svg",
              {
                xmlns: "http://www.w3.org/2000/svg",
                width: "18",
                height: "18",
                viewBox: "0 0 24 24",
                fill: "none",
                stroke: "currentColor",
                strokeWidth: "2",
                strokeLinecap: "round",
                strokeLinejoin: "round",
                className: "lucide lucide-zoom-out h-4 w-4",
                children: [
                  /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("circle", { cx: "11", cy: "11", r: "8" }),
                  /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("line", { x1: "21", x2: "16.65", y1: "21", y2: "16.65" }),
                  /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("line", { x1: "8", x2: "14", y1: "11", y2: "11" })
                ]
              }
            )
          }
        ),
        /* @__PURE__ */ (0, import_jsx_runtime68.jsxs)("span", { className: "w-12 text-center text-sm font-medium tabular-nums", children: [
          Math.round(zoom * 100),
          "%"
        ] }),
        /* @__PURE__ */ (0, import_jsx_runtime68.jsx)(
          "button",
          {
            onClick: zoomIn,
            className: "inline-flex items-center rounded-lg p-2.5 hover:bg-surface-200 dark:hover:bg-dark-surface-200",
            "aria-label": "Zoom in",
            children: /* @__PURE__ */ (0, import_jsx_runtime68.jsxs)(
              "svg",
              {
                xmlns: "http://www.w3.org/2000/svg",
                width: "18",
                height: "18",
                viewBox: "0 0 24 24",
                fill: "none",
                stroke: "currentColor",
                strokeWidth: "2",
                strokeLinecap: "round",
                strokeLinejoin: "round",
                className: "lucide lucide-zoom-in h-4 w-4",
                children: [
                  /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("circle", { cx: "11", cy: "11", r: "8" }),
                  /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("line", { x1: "21", x2: "16.65", y1: "21", y2: "16.65" }),
                  /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("line", { x1: "11", x2: "11", y1: "8", y2: "14" }),
                  /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("line", { x1: "8", x2: "14", y1: "11", y2: "11" })
                ]
              }
            )
          }
        ),
        /* @__PURE__ */ (0, import_jsx_runtime68.jsx)(
          "button",
          {
            onClick: resetZoom,
            className: "inline-flex items-center rounded-lg p-2.5 hover:bg-surface-200 dark:hover:bg-dark-surface-200",
            "aria-label": "Reset view",
            children: /* @__PURE__ */ (0, import_jsx_runtime68.jsxs)(
              "svg",
              {
                xmlns: "http://www.w3.org/2000/svg",
                width: "18",
                height: "18",
                viewBox: "0 0 24 24",
                fill: "none",
                stroke: "currentColor",
                strokeWidth: "2",
                strokeLinecap: "round",
                strokeLinejoin: "round",
                className: "lucide lucide-maximize2 h-4 w-4",
                children: [
                  /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("polyline", { points: "15 3 21 3 21 9" }),
                  /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("polyline", { points: "9 21 3 21 3 15" }),
                  /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("line", { x1: "21", x2: "14", y1: "3", y2: "10" }),
                  /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("line", { x1: "3", x2: "10", y1: "21", y2: "14" })
                ]
              }
            )
          }
        )
      ] })
    ] }),
    /* @__PURE__ */ (0, import_jsx_runtime68.jsxs)(
      "div",
      {
        ref: containerRef,
        className: cn2(
          "relative flex-1 w-full",
          scrollOnOverflow ? "overflow-auto" : "overflow-hidden",
          containerRing,
          className
        ),
        style: height != null ? { height } : void 0,
        children: [
          scrollOnOverflow ? /* @__PURE__ */ (0, import_jsx_runtime68.jsx)(
            "div",
            {
              className: "absolute",
              style: {
                width: fitBox.width * zoom,
                height: fitBox.height * zoom,
                left: 0,
                top: 0
              },
              children: /* @__PURE__ */ (0, import_jsx_runtime68.jsxs)(
                "svg",
                {
                  ref: svgRef,
                  className: "w-full h-full",
                  viewBox: `${fitBox.x} ${fitBox.y} ${fitBox.width} ${fitBox.height}`,
                  preserveAspectRatio: "xMidYMid meet",
                  children: [
                    /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("defs", { children: /* @__PURE__ */ (0, import_jsx_runtime68.jsx)(
                      "pattern",
                      {
                        id: gridId,
                        width: "20",
                        height: "20",
                        patternUnits: "userSpaceOnUse",
                        children: /* @__PURE__ */ (0, import_jsx_runtime68.jsx)(
                          "path",
                          {
                            d: "M 20 0 L 0 0 0 20",
                            fill: "none",
                            stroke: "currentColor",
                            strokeWidth: "0.5",
                            className: "text-surface-300 dark:text-dark-surface-600"
                          }
                        )
                      }
                    ) }),
                    /* @__PURE__ */ (0, import_jsx_runtime68.jsx)(
                      "rect",
                      {
                        x: fitBox.x - 1e3,
                        y: fitBox.y - 1e3,
                        width: fitBox.width + 2e3,
                        height: fitBox.height + 2e3,
                        fill: `url(#${gridId})`,
                        className: "text-surface-200 dark:text-dark-surface-700"
                      }
                    ),
                    DebugViewBoxRect,
                    DebugContentRect,
                    /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("g", { transform: "scale(1)", children })
                  ]
                }
              )
            }
          ) : /* @__PURE__ */ (0, import_jsx_runtime68.jsxs)(
            "svg",
            {
              ref: svgRef,
              className: "absolute inset-0",
              width: "100%",
              height: "100%",
              viewBox: `${viewBox.x} ${viewBox.y} ${viewBox.width} ${viewBox.height}`,
              preserveAspectRatio: "xMidYMid meet",
              children: [
                /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("defs", { children: /* @__PURE__ */ (0, import_jsx_runtime68.jsx)(
                  "pattern",
                  {
                    id: gridId,
                    width: "20",
                    height: "20",
                    patternUnits: "userSpaceOnUse",
                    children: /* @__PURE__ */ (0, import_jsx_runtime68.jsx)(
                      "path",
                      {
                        d: "M 20 0 L 0 0 0 20",
                        fill: "none",
                        stroke: "currentColor",
                        strokeWidth: "0.5",
                        className: "text-surface-300 dark:text-dark-surface-600"
                      }
                    )
                  }
                ) }),
                /* @__PURE__ */ (0, import_jsx_runtime68.jsx)(
                  "rect",
                  {
                    x: viewBox.x - 1e3,
                    y: viewBox.y - 1e3,
                    width: viewBox.width + 2e3,
                    height: viewBox.height + 2e3,
                    fill: `url(#${gridId})`,
                    className: "text-surface-200 dark:text-dark-surface-700"
                  }
                ),
                DebugViewBoxRect,
                DebugContentRect,
                /* @__PURE__ */ (0, import_jsx_runtime68.jsx)("g", { transform: `scale(${zoomForRender})`, children })
              ]
            }
          ),
          debug && /* @__PURE__ */ (0, import_jsx_runtime68.jsxs)("div", { className: "pointer-events-none absolute left-2 top-2 z-20 rounded bg-black/70 px-2 py-1 text-xs text-white", children: [
            /* @__PURE__ */ (0, import_jsx_runtime68.jsxs)("div", { children: [
              "container: ",
              Math.round(containerPx.w),
              "\xD7",
              Math.round(containerPx.h),
              "px"
            ] }),
            /* @__PURE__ */ (0, import_jsx_runtime68.jsxs)("div", { children: [
              "viewBox: ",
              Math.round(viewBox.x),
              ",",
              Math.round(viewBox.y),
              " \u2192",
              " ",
              Math.round(viewBox.width),
              "\xD7",
              Math.round(viewBox.height)
            ] }),
            /* @__PURE__ */ (0, import_jsx_runtime68.jsxs)("div", { children: [
              "zoom: ",
              Math.round(zoom * 100),
              "%"
            ] }),
            /* @__PURE__ */ (0, import_jsx_runtime68.jsxs)("div", { children: [
              "mode: ",
              scrollOnOverflow ? "scroll" : "fit"
            ] })
          ] })
        ]
      }
    )
  ] });
};

// src/components/visualization/StateVisualizer.tsx
var import_jsx_runtime69 = require("react/jsx-runtime");
var DEFAULT_LABELS2 = {
  taskId: "Task",
  taskType: "Type",
  inputType: "Input",
  outputType: "Output",
  transition: "Transition",
  duration: "Duration",
  error: "Error"
};
var StateVisualizer = ({ state, labels }) => {
  if (!state || state.length === 0) {
    return null;
  }
  const l = { ...DEFAULT_LABELS2, ...labels };
  const formatDuration = (ms) => {
    if (ms < 1e3) return `${Math.round(ms)} ms`;
    return `${(ms / 1e3).toFixed(2)} s`;
  };
  return /* @__PURE__ */ (0, import_jsx_runtime69.jsx)(
    Table,
    {
      columns: [
        l.taskId,
        l.taskType,
        l.inputType,
        l.outputType,
        l.transition,
        l.duration,
        l.error
      ],
      children: state.map((unit, index) => /* @__PURE__ */ (0, import_jsx_runtime69.jsxs)(TableRow, { className: unit.error ? "bg-error/10" : "", children: [
        /* @__PURE__ */ (0, import_jsx_runtime69.jsx)(TableCell, { children: unit.taskID }),
        /* @__PURE__ */ (0, import_jsx_runtime69.jsx)(TableCell, { children: unit.taskType }),
        /* @__PURE__ */ (0, import_jsx_runtime69.jsx)(TableCell, { children: unit.inputType }),
        /* @__PURE__ */ (0, import_jsx_runtime69.jsx)(TableCell, { children: unit.outputType }),
        /* @__PURE__ */ (0, import_jsx_runtime69.jsx)(TableCell, { className: "max-w-xs truncate", children: unit.transition || "-" }),
        /* @__PURE__ */ (0, import_jsx_runtime69.jsx)(TableCell, { children: formatDuration(unit.duration) }),
        /* @__PURE__ */ (0, import_jsx_runtime69.jsx)(TableCell, { children: unit.error?.error ? /* @__PURE__ */ (0, import_jsx_runtime69.jsx)(Badge, { variant: "error", size: "sm", children: unit.error.error }) : "-" })
      ] }, index))
    }
  );
};

// src/components/TerminalOutput.tsx
var import_react35 = require("react");
var import_jsx_runtime70 = require("react/jsx-runtime");
var TerminalOutput = (0, import_react35.forwardRef)(
  function TerminalOutput2({
    className,
    lines,
    autoScroll = true,
    title,
    actions,
    maxHeight = "100%",
    ...props
  }, ref) {
    const scrollRef = (0, import_react35.useRef)(null);
    (0, import_react35.useEffect)(() => {
      if (autoScroll && scrollRef.current) {
        scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
      }
    }, [lines, autoScroll]);
    return /* @__PURE__ */ (0, import_jsx_runtime70.jsxs)(
      "div",
      {
        ref,
        className: cn(
          "flex flex-col overflow-hidden rounded-lg border",
          "border-surface-300 dark:border-dark-surface-500",
          "bg-surface-900 dark:bg-dark-surface-50",
          "text-surface-100 dark:text-dark-text",
          className
        ),
        style: { maxHeight },
        ...props,
        children: [
          (title || actions) && /* @__PURE__ */ (0, import_jsx_runtime70.jsxs)(
            "div",
            {
              className: cn(
                "flex shrink-0 items-center justify-between gap-2 border-b px-3 py-1.5",
                "border-surface-700 dark:border-dark-surface-300",
                "bg-surface-800 dark:bg-dark-surface-100"
              ),
              children: [
                title && /* @__PURE__ */ (0, import_jsx_runtime70.jsx)("span", { className: "text-xs font-medium text-surface-300 dark:text-dark-text-muted", children: title }),
                actions && /* @__PURE__ */ (0, import_jsx_runtime70.jsx)("div", { className: "flex items-center gap-1", children: actions })
              ]
            }
          ),
          /* @__PURE__ */ (0, import_jsx_runtime70.jsx)(
            "div",
            {
              ref: scrollRef,
              className: "flex-1 overflow-auto p-3",
              children: /* @__PURE__ */ (0, import_jsx_runtime70.jsx)("pre", { className: "whitespace-pre-wrap break-all font-mono text-xs leading-5", children: lines.map((line, i) => /* @__PURE__ */ (0, import_jsx_runtime70.jsxs)("div", { children: [
                colorize(line),
                "\n"
              ] }, i)) })
            }
          )
        ]
      }
    );
  }
);
var ANSI_CLASSES = {
  "30": "text-surface-900 dark:text-dark-surface-900",
  // black
  "31": "text-error dark:text-dark-error",
  // red
  "32": "text-success dark:text-dark-success",
  // green
  "33": "text-warning dark:text-dark-warning",
  // yellow
  "34": "text-primary dark:text-dark-primary",
  // blue
  "35": "text-accent dark:text-dark-accent",
  // magenta → accent
  "36": "text-info dark:text-dark-info",
  // cyan → info
  "37": "text-surface-100 dark:text-dark-text",
  // white
  "1": "font-bold",
  "2": "opacity-60",
  "3": "italic",
  "4": "underline"
};
var ANSI_RE = /\x1b\[([0-9;]*)m/g;
function colorize(text) {
  if (!text.includes("\x1B[")) return text;
  const parts = [];
  let lastIndex = 0;
  let activeClasses = "";
  let match;
  ANSI_RE.lastIndex = 0;
  while ((match = ANSI_RE.exec(text)) !== null) {
    if (match.index > lastIndex) {
      const chunk = text.slice(lastIndex, match.index);
      parts.push(
        activeClasses ? /* @__PURE__ */ (0, import_jsx_runtime70.jsx)("span", { className: activeClasses, children: chunk }, parts.length) : chunk
      );
    }
    lastIndex = match.index + match[0].length;
    const codes = match[1].split(";");
    for (const code of codes) {
      if (code === "0" || code === "") {
        activeClasses = "";
      } else if (ANSI_CLASSES[code]) {
        activeClasses = activeClasses ? `${activeClasses} ${ANSI_CLASSES[code]}` : ANSI_CLASSES[code];
      }
    }
  }
  if (lastIndex < text.length) {
    const chunk = text.slice(lastIndex);
    parts.push(
      activeClasses ? /* @__PURE__ */ (0, import_jsx_runtime70.jsx)("span", { className: activeClasses, children: chunk }, parts.length) : chunk
    );
  }
  return parts.length === 1 ? parts[0] : /* @__PURE__ */ (0, import_jsx_runtime70.jsx)(import_jsx_runtime70.Fragment, { children: parts });
}

// src/components/visualization/TaskEventFeed.tsx
var import_jsx_runtime71 = require("react/jsx-runtime");
var TAIL = 40;
function extrasLine(e) {
  const parts = [];
  if (e.request_id) parts.push(`request=${e.request_id}`);
  if (e.chain_id) parts.push(`chain=${e.chain_id}`);
  if (e.task_handler) parts.push(`handler=${e.task_handler}`);
  if (e.model_name) parts.push(`model=${e.model_name}`);
  if (e.transition) parts.push(`transition=${e.transition}`);
  if (e.content && e.kind === "step_chunk") {
    const c = e.content.trim().replace(/\s+/g, " ");
    parts.push(c.length > 120 ? `${c.slice(0, 120)}\u2026` : c);
  }
  return parts.length ? parts.join(" ") : null;
}
function eventToLines(e) {
  const base = `[${e.timestamp}] ${e.kind}${e.task_id ? ` ${e.task_id}` : ""}`;
  const lines = [base];
  const extra = extrasLine(e);
  if (extra) lines.push(`  ${extra}`);
  if (e.error) lines.push(`  error: ${e.error}`);
  return lines;
}
var DEFAULT_OMITTED_LABEL = (count) => `\u2026 ${count} earlier event${count === 1 ? "" : "s"} omitted`;
function TaskEventFeed({ events, omittedLabel = DEFAULT_OMITTED_LABEL }) {
  if (!events.length) return null;
  const tail = events.length > TAIL ? events.slice(-TAIL) : events;
  const omitted = events.length - tail.length;
  const lines = tail.flatMap(eventToLines);
  return /* @__PURE__ */ (0, import_jsx_runtime71.jsxs)("div", { className: "flex flex-col gap-1", children: [
    omitted > 0 ? /* @__PURE__ */ (0, import_jsx_runtime71.jsx)(Span, { variant: "muted", className: "text-[10px]", children: omittedLabel(omitted) }) : null,
    /* @__PURE__ */ (0, import_jsx_runtime71.jsx)(TerminalOutput, { lines, maxHeight: "min(320px, 40vh)", className: "min-h-[120px]" })
  ] });
}

// src/components/visualization/ExecutionTimeline.tsx
var import_react36 = require("react");
var import_lucide_react15 = require("lucide-react");
var import_jsx_runtime72 = require("react/jsx-runtime");
var DEFAULT_LABELS3 = {
  executionLog: "Execution Log",
  initializingPlan: "Initializing Plan",
  awaitingApproval: "Awaiting Approval",
  showState: (count) => `Show State Logs (${count})`,
  taskId: "Task",
  taskType: "Type",
  transition: "Transition",
  duration: "Duration",
  error: "Error"
};
function ExecutionTimeline({ events, state, labels }) {
  if ((!events || events.length === 0) && (!state || state.length === 0)) {
    return null;
  }
  const l = { ...DEFAULT_LABELS3, ...labels };
  return /* @__PURE__ */ (0, import_jsx_runtime72.jsxs)("div", { className: "flex flex-col gap-2 pt-3 border-t border-surface-300 dark:border-dark-surface-400", children: [
    events && events.length > 0 && /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(LiveTaskEvents, { events, l }),
    state && state.length > 0 && (!events || events.length === 0) && /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(HistoricalState, { state, l })
  ] });
}
function LiveTaskEvents({
  events,
  l
}) {
  const steps = (0, import_react36.useMemo)(() => {
    const groups = [];
    let currentId = null;
    for (const e of events) {
      const stepId = e.task_id || e.task_handler || "system";
      if (stepId !== currentId) {
        currentId = stepId;
        groups.push({ id: stepId, events: [] });
      }
      groups[groups.length - 1].events.push(e);
    }
    return groups;
  }, [events]);
  return /* @__PURE__ */ (0, import_jsx_runtime72.jsxs)("div", { className: "flex flex-col gap-2 text-sm", children: [
    /* @__PURE__ */ (0, import_jsx_runtime72.jsxs)("div", { className: "flex items-center gap-2 text-text-muted font-medium px-1", children: [
      /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(import_lucide_react15.Activity, { size: 14 }),
      /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(Span, { children: l.executionLog })
    ] }),
    steps.map((group, idx) => /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(StepCollapsible, { group, l }, `${group.id}-${idx}`))
  ] });
}
function StepCollapsible({
  group,
  l
}) {
  const events = group.events;
  const isError = events.some((e) => e.kind === "step_failed" || e.kind === "chain_failed");
  const isDone = events.some((e) => e.kind === "step_completed" || e.kind === "chain_completed");
  const transitionEvent = events.find((e) => !!e.transition);
  let title = group.id;
  if (title === "system" && events.some((e) => e.kind === "chain_started")) {
    title = l.initializingPlan;
  } else if (events.some((e) => e.kind === "approval_requested")) {
    title = l.awaitingApproval;
  }
  const TitleElement = /* @__PURE__ */ (0, import_jsx_runtime72.jsxs)("div", { className: "flex items-center gap-2", children: [
    /* @__PURE__ */ (0, import_jsx_runtime72.jsx)("span", { className: "flex-shrink-0", children: isError ? /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(import_lucide_react15.AlertCircle, { size: 14, className: "text-error" }) : isDone ? /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(import_lucide_react15.CheckCircle2, { size: 14, className: "text-success" }) : /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(import_lucide_react15.Settings, { size: 14, className: "text-text-muted dark:text-dark-text-muted animate-spin-slow" }) }),
    /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(Span, { className: "font-mono text-xs font-semibold", children: title }),
    transitionEvent && transitionEvent.transition && /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(Badge, { variant: "outline", size: "sm", className: "text-[10px] py-0", children: transitionEvent.transition })
  ] });
  return /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(Collapsible, { title: TitleElement, className: "bg-background", children: /* @__PURE__ */ (0, import_jsx_runtime72.jsx)("div", { className: "p-3 font-mono text-[11px] overflow-x-auto whitespace-pre bg-surface-50 dark:bg-dark-surface-50 rounded-b-md", children: events.map((e, idx) => /* @__PURE__ */ (0, import_jsx_runtime72.jsxs)("div", { className: "flex gap-2", children: [
    /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(Span, { className: "text-text-muted opacity-50 shrink-0", children: new Date(e.timestamp).toLocaleTimeString([], { hour12: false }) }),
    /* @__PURE__ */ (0, import_jsx_runtime72.jsxs)(Span, { className: cn(e.error ? "text-error font-medium" : "text-text dark:text-dark-text"), children: [
      e.kind,
      e.task_handler && e.task_handler !== group.id ? ` [${e.task_handler}]` : "",
      e.error ? ` - ${e.error}` : ""
    ] })
  ] }, idx)) }) });
}
function HistoricalState({
  state,
  l
}) {
  const formatDuration = (ms) => {
    if (ms < 1e3) return `${Math.round(ms)} ms`;
    return `${(ms / 1e3).toFixed(2)} s`;
  };
  const TitleElement = /* @__PURE__ */ (0, import_jsx_runtime72.jsxs)("div", { className: "flex items-center gap-2", children: [
    /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(import_lucide_react15.Activity, { size: 14 }),
    /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(Span, { className: "font-medium text-xs", children: l.showState(state.length) })
  ] });
  return /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(Collapsible, { title: TitleElement, className: "mt-1", children: /* @__PURE__ */ (0, import_jsx_runtime72.jsx)("div", { className: "border border-surface-300 dark:border-dark-surface-400 rounded-b-md overflow-x-auto", children: /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(
    Table,
    {
      columns: [l.taskId, l.taskType, l.transition, l.duration, l.error],
      children: state.map((unit, index) => /* @__PURE__ */ (0, import_jsx_runtime72.jsxs)(TableRow, { children: [
        /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(TableCell, { className: "font-mono text-xs", children: unit.taskID }),
        /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(TableCell, { className: "text-xs", children: unit.taskType }),
        /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(TableCell, { className: "max-w-xs truncate text-xs", children: unit.transition || "-" }),
        /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(TableCell, { className: "text-xs", children: formatDuration(unit.duration) }),
        /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(TableCell, { className: "text-xs", children: unit.error?.error ? /* @__PURE__ */ (0, import_jsx_runtime72.jsx)(Badge, { variant: "error", size: "sm", children: unit.error.error }) : "-" })
      ] }, index))
    }
  ) }) });
}

// src/components/editors/JsonEditor.tsx
var import_react37 = require("react");
var import_jsx_runtime73 = require("react/jsx-runtime");
var JsonEditor = ({
  value,
  onSave,
  onCancel,
  title = "JSON Editor",
  description = "Edit the JSON data below. Use the format button to automatically format the JSON.",
  validate,
  exampleJson = `{
  "key": "value"
}`,
  className
}) => {
  const [jsonInput, setJsonInput] = (0, import_react37.useState)("");
  const [error, setError] = (0, import_react37.useState)(void 0);
  const [isValid, setIsValid] = (0, import_react37.useState)(true);
  (0, import_react37.useEffect)(() => {
    try {
      setJsonInput(JSON.stringify(value, null, 2));
      setError(void 0);
      setIsValid(true);
    } catch {
      setError("Failed to initialize JSON editor");
      setIsValid(false);
    }
  }, [value]);
  const validateJson = (jsonString) => {
    try {
      if (!jsonString.trim()) {
        setError("JSON cannot be empty");
        return false;
      }
      const parsed = JSON.parse(jsonString);
      if (validate) {
        const res = validate(parsed);
        if (!res.isValid) {
          setError(res.error || "Validation failed");
          return false;
        }
      }
      setError(void 0);
      return true;
    } catch (err) {
      setError(`Invalid JSON: ${err.message}`);
      return false;
    }
  };
  const handleJsonChange = (value2) => {
    setJsonInput(value2);
    setIsValid(validateJson(value2));
  };
  const handleSave = () => {
    if (!validateJson(jsonInput)) return;
    try {
      const parsedJson = JSON.parse(jsonInput);
      onSave(parsedJson);
    } catch (err) {
      setError(`Failed to save JSON: ${err.message}`);
    }
  };
  const handleFormat = () => {
    try {
      const parsed = JSON.parse(jsonInput);
      setJsonInput(JSON.stringify(parsed, null, 2));
      setIsValid(true);
      setError(void 0);
    } catch (err) {
      setError(`Failed to format JSON: ${err.message}`);
      setIsValid(false);
    }
  };
  return /* @__PURE__ */ (0, import_jsx_runtime73.jsxs)(
    GridLayout,
    {
      className: cn("h-full min-h-0 gap-4", className),
      responsive: { base: 1, lg: 2 },
      children: [
        /* @__PURE__ */ (0, import_jsx_runtime73.jsxs)(Card, { className: "flex h-full min-h-0 flex-col overflow-hidden p-4 gap-4", children: [
          /* @__PURE__ */ (0, import_jsx_runtime73.jsxs)("div", { className: "flex-shrink-0 space-y-2", children: [
            /* @__PURE__ */ (0, import_jsx_runtime73.jsx)("h3", { className: "text-lg font-semibold text-text dark:text-dark-text", children: title }),
            /* @__PURE__ */ (0, import_jsx_runtime73.jsx)("p", { className: "text-text-muted text-sm dark:text-dark-text-muted", children: description }),
            error && /* @__PURE__ */ (0, import_jsx_runtime73.jsx)(Panel, { variant: "error", children: error })
          ] }),
          /* @__PURE__ */ (0, import_jsx_runtime73.jsx)(
            FormField,
            {
              label: "JSON Data",
              error,
              className: "flex min-h-0 flex-1 flex-col",
              children: /* @__PURE__ */ (0, import_jsx_runtime73.jsxs)("div", { className: "flex min-h-0 min-w-0 flex-1 flex-col", children: [
                /* @__PURE__ */ (0, import_jsx_runtime73.jsxs)("div", { className: "flex items-center justify-between", children: [
                  /* @__PURE__ */ (0, import_jsx_runtime73.jsx)("span", { className: "text-sm font-medium text-text dark:text-dark-text", children: "JSON" }),
                  /* @__PURE__ */ (0, import_jsx_runtime73.jsx)(Button, { size: "sm", variant: "outline", onClick: handleFormat, children: "Format JSON" })
                ] }),
                /* @__PURE__ */ (0, import_jsx_runtime73.jsx)("div", { className: "relative min-h-0 min-w-0 flex-1 overflow-hidden rounded-lg border border-surface-300 dark:border-dark-surface-300", children: /* @__PURE__ */ (0, import_jsx_runtime73.jsx)(
                  Textarea,
                  {
                    value: jsonInput,
                    onChange: (e) => handleJsonChange(e.target.value),
                    className: "h-full w-full font-mono text-sm whitespace-pre overflow-auto",
                    placeholder: "Enter JSON here..."
                  }
                ) })
              ] })
            }
          ),
          /* @__PURE__ */ (0, import_jsx_runtime73.jsxs)("div", { className: "flex flex-shrink-0 items-center justify-between border-t border-surface-300 pt-4 dark:border-dark-surface-300", children: [
            /* @__PURE__ */ (0, import_jsx_runtime73.jsx)("div", { className: "flex items-center gap-2", children: isValid ? /* @__PURE__ */ (0, import_jsx_runtime73.jsx)("span", { className: "text-success-500 dark:text-dark-success-500 text-sm", children: "\u2713 Valid JSON" }) : /* @__PURE__ */ (0, import_jsx_runtime73.jsx)("span", { className: "text-error-500 dark:text-dark-error-500 text-sm", children: "\u2717 Invalid JSON" }) }),
            /* @__PURE__ */ (0, import_jsx_runtime73.jsxs)("div", { className: "flex gap-2", children: [
              /* @__PURE__ */ (0, import_jsx_runtime73.jsx)(Button, { variant: "secondary", onClick: onCancel, children: "Cancel" }),
              /* @__PURE__ */ (0, import_jsx_runtime73.jsx)(Button, { variant: "primary", onClick: handleSave, disabled: !isValid, children: "Save" })
            ] })
          ] })
        ] }),
        /* @__PURE__ */ (0, import_jsx_runtime73.jsx)("div", { className: "flex h-full min-h-0 flex-col overflow-hidden", children: /* @__PURE__ */ (0, import_jsx_runtime73.jsxs)(
          Panel,
          {
            variant: "surface",
            className: "flex h-full min-h-0 flex-col overflow-hidden p-4 gap-2",
            children: [
              /* @__PURE__ */ (0, import_jsx_runtime73.jsx)("h4", { className: "flex-shrink-0 font-medium text-text dark:text-dark-text", children: "Example JSON Structure" }),
              /* @__PURE__ */ (0, import_jsx_runtime73.jsx)("div", { className: "min-h-0 flex-1 overflow-hidden", children: /* @__PURE__ */ (0, import_jsx_runtime73.jsx)("pre", { className: "min-h-full rounded bg-surface-100 p-3 font-mono text-xs text-text dark:bg-dark-surface-100 dark:text-dark-text", children: exampleJson }) })
            ]
          }
        ) })
      ]
    }
  );
};
var JsonEditor_default = JsonEditor;

// src/components/wizard/Wizard.tsx
var import_jsx_runtime74 = require("react/jsx-runtime");
function Wizard({
  title,
  description,
  footer,
  children,
  className
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime74.jsxs)(
    Panel,
    {
      variant: "bordered",
      className: cn(
        "border-amber-500/60 bg-amber-500/5 text-inherit shrink-0 px-4 py-4",
        className
      ),
      children: [
        (title || description) && /* @__PURE__ */ (0, import_jsx_runtime74.jsxs)("header", { className: "mb-4 space-y-1", children: [
          title ? /* @__PURE__ */ (0, import_jsx_runtime74.jsx)(H3, { variant: "subsectionTitle", className: "text-balance", children: title }) : null,
          description ? /* @__PURE__ */ (0, import_jsx_runtime74.jsx)(P, { variant: "muted", className: "text-sm", children: description }) : null
        ] }),
        /* @__PURE__ */ (0, import_jsx_runtime74.jsx)("div", { className: "space-y-0", children }),
        footer ? /* @__PURE__ */ (0, import_jsx_runtime74.jsx)("footer", { className: "mt-4 border-t border-surface-200 pt-4 dark:border-dark-surface-700", children: footer }) : null
      ]
    }
  );
}

// src/components/wizard/WizardStep.tsx
var import_lucide_react16 = require("lucide-react");
var import_jsx_runtime75 = require("react/jsx-runtime");
function StepIndicator({
  step,
  status
}) {
  const base = "flex h-9 w-9 shrink-0 items-center justify-center rounded-full border-2 text-sm font-semibold transition-colors";
  if (status === "complete") {
    return /* @__PURE__ */ (0, import_jsx_runtime75.jsx)(
      "span",
      {
        className: cn(
          base,
          "border-primary-600 bg-primary-600 text-white dark:border-dark-primary-500 dark:bg-dark-primary-500 dark:text-dark-surface-50"
        ),
        "aria-hidden": true,
        children: /* @__PURE__ */ (0, import_jsx_runtime75.jsx)(import_lucide_react16.Check, { className: "h-4 w-4", strokeWidth: 2.5 })
      }
    );
  }
  if (status === "error") {
    return /* @__PURE__ */ (0, import_jsx_runtime75.jsx)(
      "span",
      {
        className: cn(
          base,
          "border-error bg-error/10 text-error dark:border-red-500 dark:text-red-400"
        ),
        "aria-hidden": true,
        children: /* @__PURE__ */ (0, import_jsx_runtime75.jsx)(import_lucide_react16.AlertCircle, { className: "h-4 w-4" })
      }
    );
  }
  if (status === "current") {
    return /* @__PURE__ */ (0, import_jsx_runtime75.jsx)(
      "span",
      {
        className: cn(
          base,
          "border-primary-600 bg-primary-50 text-primary-700 ring-2 ring-primary-200 ring-offset-2 ring-offset-surface-500/5 dark:border-primary-400 dark:bg-primary-900 dark:text-primary-200 dark:ring-primary-800"
        ),
        "aria-hidden": true,
        children: step
      }
    );
  }
  return /* @__PURE__ */ (0, import_jsx_runtime75.jsx)(
    "span",
    {
      className: cn(
        base,
        "border-surface-300 bg-surface-50 text-text-muted dark:text-dark-text-muted dark:border-dark-surface-600 dark:bg-dark-surface-800"
      ),
      "aria-hidden": true,
      children: step
    }
  );
}
function WizardStep({
  step,
  status,
  active = false,
  title,
  description,
  children,
  isLast = false,
  className
}) {
  const lineClass = status === "complete" ? "bg-primary-500/50 dark:bg-dark-primary-500/40" : "bg-surface-200 dark:bg-dark-surface-600";
  return /* @__PURE__ */ (0, import_jsx_runtime75.jsxs)(
    "div",
    {
      className: cn("flex gap-4", className),
      "aria-current": active || status === "current" ? "step" : void 0,
      children: [
        /* @__PURE__ */ (0, import_jsx_runtime75.jsxs)("div", { className: "flex w-9 shrink-0 flex-col items-center", children: [
          /* @__PURE__ */ (0, import_jsx_runtime75.jsx)(StepIndicator, { step, status }),
          !isLast ? /* @__PURE__ */ (0, import_jsx_runtime75.jsx)(
            "div",
            {
              className: cn("mt-2 w-px flex-1 min-h-[1.5rem]", lineClass),
              "aria-hidden": true
            }
          ) : null
        ] }),
        /* @__PURE__ */ (0, import_jsx_runtime75.jsxs)("div", { className: cn("min-w-0 flex-1", !isLast && "pb-6"), children: [
          /* @__PURE__ */ (0, import_jsx_runtime75.jsx)(
            H3,
            {
              variant: "subsectionTitle",
              className: cn(
                "text-base",
                status === "complete" && "text-text-muted dark:text-dark-text-muted",
                status === "upcoming" && "text-text-muted dark:text-dark-text-muted"
              ),
              children: title
            }
          ),
          description ? /* @__PURE__ */ (0, import_jsx_runtime75.jsx)(P, { variant: "muted", className: "mt-1 text-sm", children: description }) : null,
          children ? /* @__PURE__ */ (0, import_jsx_runtime75.jsx)("div", { className: "mt-3 space-y-2", children }) : null
        ] })
      ]
    }
  );
}

// src/components/Page.tsx
var import_jsx_runtime76 = require("react/jsx-runtime");
function Page({
  header,
  footer,
  children,
  className,
  bodyScroll = "auto"
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime76.jsxs)("div", { className: cn("flex h-full min-h-0 flex-col", className), children: [
    header && /* @__PURE__ */ (0, import_jsx_runtime76.jsx)("div", { className: "shrink-0", children: header }),
    /* @__PURE__ */ (0, import_jsx_runtime76.jsx)(
      "div",
      {
        className: cn(
          "flex min-h-0 w-full max-w-full min-w-0 flex-1 flex-col overflow-x-clip",
          bodyScroll === "auto" ? "overflow-y-auto" : "overflow-y-hidden"
        ),
        children
      }
    ),
    footer && /* @__PURE__ */ (0, import_jsx_runtime76.jsx)("div", { className: "shrink-0", children: footer })
  ] });
}
function Fill({
  children,
  className
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime76.jsx)("div", { className: cn("relative min-h-0 min-w-0 flex-1", className), children });
}

// src/components/LoadingState.tsx
var import_jsx_runtime77 = require("react/jsx-runtime");
function LoadingState({
  message = "Loading...",
  className
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime77.jsx)("div", { className: cn("flex items-center justify-center py-12", className), children: /* @__PURE__ */ (0, import_jsx_runtime77.jsxs)("div", { className: "text-center space-y-4", children: [
    /* @__PURE__ */ (0, import_jsx_runtime77.jsx)(Spinner, { size: "lg", className: "mx-auto" }),
    /* @__PURE__ */ (0, import_jsx_runtime77.jsx)(P, { variant: "muted", children: message })
  ] }) });
}
function ErrorState({
  error,
  onRetry,
  title = "Error",
  description = "An error occurred while loading data."
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime77.jsx)(Panel, { variant: "error", className: "p-6", children: /* @__PURE__ */ (0, import_jsx_runtime77.jsxs)("div", { className: "text-center space-y-4", children: [
    /* @__PURE__ */ (0, import_jsx_runtime77.jsx)(P, { className: "font-medium", children: title }),
    /* @__PURE__ */ (0, import_jsx_runtime77.jsx)(P, { variant: "muted", children: typeof error === "string" ? error : error?.message || description }),
    onRetry && /* @__PURE__ */ (0, import_jsx_runtime77.jsx)(Button, { variant: "outline", onClick: onRetry, children: "Try Again" })
  ] }) });
}

// src/components/ResourceCard.tsx
var import_jsx_runtime78 = require("react/jsx-runtime");
var statusBorderStyles = {
  default: "border-l-4 border-l-border dark:border-l-dark-surface-600",
  success: "border-l-4 border-l-success dark:border-l-dark-success",
  error: "border-l-4 border-l-error dark:border-l-dark-error",
  warning: "border-l-4 border-l-warning dark:border-l-dark-warning"
};
function ResourceCard({
  title,
  subtitle,
  status = "default",
  children,
  actions,
  isLoading = false,
  className = ""
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime78.jsxs)(
    Section,
    {
      title,
      className: `bg-surface dark:bg-dark-surface-100 relative rounded-lg ${statusBorderStyles[status]} ${className}`,
      children: [
        subtitle && /* @__PURE__ */ (0, import_jsx_runtime78.jsx)(P, { variant: "muted", className: "text-sm", children: subtitle }),
        /* @__PURE__ */ (0, import_jsx_runtime78.jsx)("div", { className: "space-y-4", children }),
        actions && /* @__PURE__ */ (0, import_jsx_runtime78.jsx)("div", { className: "border-t pt-4", children: /* @__PURE__ */ (0, import_jsx_runtime78.jsxs)(ButtonGroup, { className: "flex items-center justify-between", children: [
          /* @__PURE__ */ (0, import_jsx_runtime78.jsxs)("div", { className: "flex gap-2", children: [
            actions.edit && /* @__PURE__ */ (0, import_jsx_runtime78.jsx)(
              Button,
              {
                variant: "outline",
                size: "sm",
                onClick: actions.edit,
                disabled: isLoading,
                children: "Edit"
              }
            ),
            actions.custom
          ] }),
          actions.delete && /* @__PURE__ */ (0, import_jsx_runtime78.jsx)(
            Button,
            {
              variant: "danger",
              size: "sm",
              onClick: actions.delete,
              disabled: isLoading,
              children: isLoading ? /* @__PURE__ */ (0, import_jsx_runtime78.jsxs)(import_jsx_runtime78.Fragment, { children: [
                /* @__PURE__ */ (0, import_jsx_runtime78.jsx)(Spinner, { size: "sm" }),
                "Deleting"
              ] }) : "Delete"
            }
          )
        ] }) })
      ]
    }
  );
}

// src/components/ResizablePanel.tsx
var import_react38 = require("react");
var import_jsx_runtime79 = require("react/jsx-runtime");
var ResizablePanelGroup = (0, import_react38.forwardRef)(function ResizablePanelGroup2({ className, orientation = "horizontal", ...props }, ref) {
  return /* @__PURE__ */ (0, import_jsx_runtime79.jsx)(
    "div",
    {
      ref,
      "data-orientation": orientation,
      className: cn(
        "flex min-h-0 min-w-0",
        orientation === "horizontal" ? "flex-row" : "flex-col",
        className
      ),
      ...props
    }
  );
});
var ResizablePanel = (0, import_react38.forwardRef)(
  function ResizablePanel2({ className, defaultSize, minSize, maxSize, style, ...props }, ref) {
    return /* @__PURE__ */ (0, import_jsx_runtime79.jsx)(
      "div",
      {
        ref,
        className: cn(
          // not overflow-auto: scroll containers break flex sizing for children (e.g. xterm FitAddon)
          "min-h-0 min-w-0 overflow-hidden",
          className
        ),
        style: {
          flexBasis: defaultSize,
          flexGrow: defaultSize ? 0 : 1,
          flexShrink: defaultSize ? 0 : 1,
          minWidth: minSize,
          maxWidth: maxSize,
          ...style
        },
        ...props
      }
    );
  }
);
var ResizablePanelHandle = (0, import_react38.forwardRef)(function ResizablePanelHandle2({ className, orientation = "horizontal", onResize, onResizeEnd, ...props }, ref) {
  const innerRef = (0, import_react38.useRef)(null);
  const [dragging, setDragging] = (0, import_react38.useState)(false);
  const lastPos = (0, import_react38.useRef)(0);
  const assignRef = (0, import_react38.useCallback)(
    (el) => {
      innerRef.current = el;
      if (typeof ref === "function") ref(el);
      else if (ref) ref.current = el;
    },
    [ref]
  );
  const handlePointerDown = (0, import_react38.useCallback)(
    (e) => {
      e.preventDefault();
      setDragging(true);
      lastPos.current = orientation === "horizontal" ? e.clientX : e.clientY;
      e.target.setPointerCapture(e.pointerId);
    },
    [orientation]
  );
  const handlePointerMove = (0, import_react38.useCallback)(
    (e) => {
      if (!dragging) return;
      const current = orientation === "horizontal" ? e.clientX : e.clientY;
      const delta = current - lastPos.current;
      lastPos.current = current;
      const next = innerRef.current?.nextElementSibling;
      if (next && delta !== 0) {
        const currentSize = orientation === "horizontal" ? next.getBoundingClientRect().width : next.getBoundingClientRect().height;
        const newSize = currentSize - delta;
        next.style.flexBasis = `${newSize}px`;
        next.style.flexGrow = "0";
        next.style.flexShrink = "0";
        onResize?.(delta);
      }
    },
    [dragging, orientation, onResize]
  );
  const handlePointerUp = (0, import_react38.useCallback)(() => {
    setDragging(false);
    onResizeEnd?.();
  }, [onResizeEnd]);
  (0, import_react38.useEffect)(() => {
    if (!dragging) return;
    const up = () => {
      setDragging(false);
      onResizeEnd?.();
    };
    window.addEventListener("pointerup", up);
    return () => window.removeEventListener("pointerup", up);
  }, [dragging, onResizeEnd]);
  const isHorizontal = orientation === "horizontal";
  return /* @__PURE__ */ (0, import_jsx_runtime79.jsx)(
    "div",
    {
      ref: assignRef,
      role: "separator",
      "aria-orientation": orientation,
      tabIndex: 0,
      className: cn(
        "flex shrink-0 items-center justify-center",
        "bg-surface-200 dark:bg-dark-surface-400",
        "hover:bg-surface-300 dark:hover:bg-dark-surface-500",
        "active:bg-surface-400 dark:active:bg-dark-surface-600",
        "transition-colors",
        isHorizontal ? "w-1.5 cursor-col-resize" : "h-1.5 cursor-row-resize",
        dragging && (isHorizontal ? "bg-surface-400 dark:bg-dark-surface-600" : "bg-surface-400 dark:bg-dark-surface-600"),
        className
      ),
      onPointerDown: handlePointerDown,
      onPointerMove: handlePointerMove,
      onPointerUp: handlePointerUp,
      ...props,
      children: /* @__PURE__ */ (0, import_jsx_runtime79.jsx)(
        "div",
        {
          className: cn(
            "rounded-full bg-surface-400 dark:bg-dark-surface-600",
            isHorizontal ? "h-6 w-0.5" : "h-0.5 w-6"
          )
        }
      )
    }
  );
});

// src/components/FileTree.tsx
var import_react39 = require("react");
var import_lucide_react17 = require("lucide-react");
var import_jsx_runtime80 = require("react/jsx-runtime");
var FileTree = (0, import_react39.forwardRef)(
  function FileTree2({
    className,
    nodes,
    selectedId,
    onNodeSelect,
    directoryClickMode = "expand",
    defaultExpanded,
    indent = 16,
    ...props
  }, ref) {
    return /* @__PURE__ */ (0, import_jsx_runtime80.jsx)(
      "div",
      {
        ref,
        role: "tree",
        className: cn("text-sm select-none", className),
        ...props,
        children: nodes.map((node) => /* @__PURE__ */ (0, import_jsx_runtime80.jsx)(
          FileTreeItem,
          {
            node,
            depth: 0,
            indent,
            selectedId,
            onNodeSelect,
            directoryClickMode,
            defaultExpanded
          },
          node.id
        ))
      }
    );
  }
);
function FileTreeItem({
  node,
  depth,
  indent,
  selectedId,
  onNodeSelect,
  directoryClickMode,
  defaultExpanded
}) {
  const [expanded, setExpanded] = (0, import_react39.useState)(
    () => defaultExpanded?.has(node.id) ?? node.isDirectory === true
  );
  const isSelected = selectedId === node.id;
  const toggleExpand = (0, import_react39.useCallback)(() => {
    setExpanded((v) => !v);
  }, []);
  const handleRowClick = (0, import_react39.useCallback)(() => {
    if (node.isDirectory && directoryClickMode === "expand") {
      setExpanded((v) => !v);
    }
    onNodeSelect?.(node);
  }, [node, onNodeSelect, directoryClickMode]);
  const handleChevronClick = (0, import_react39.useCallback)(
    (e) => {
      e.stopPropagation();
      toggleExpand();
    },
    [toggleExpand]
  );
  const handleKeyDown = (0, import_react39.useCallback)(
    (e) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        handleRowClick();
      }
      if (node.isDirectory) {
        if (e.key === "ArrowRight" && !expanded) {
          e.preventDefault();
          setExpanded(true);
        }
        if (e.key === "ArrowLeft" && expanded) {
          e.preventDefault();
          setExpanded(false);
        }
      }
    },
    [handleRowClick, node.isDirectory, expanded]
  );
  const rowShellClass = cn(
    "flex w-full items-center gap-1.5 rounded px-2 py-1 text-left",
    "text-text dark:text-dark-text",
    "hover:bg-surface-100 dark:hover:bg-dark-surface-200",
    isSelected && "bg-primary-50/50 text-primary-700 dark:bg-dark-primary-900/30 dark:text-dark-primary-400"
  );
  return /* @__PURE__ */ (0, import_jsx_runtime80.jsxs)("div", { role: "treeitem", "aria-expanded": node.isDirectory ? expanded : void 0, children: [
    node.isDirectory && directoryClickMode === "navigate" ? /* @__PURE__ */ (0, import_jsx_runtime80.jsxs)("div", { className: rowShellClass, style: { paddingLeft: depth * indent + 8 }, children: [
      /* @__PURE__ */ (0, import_jsx_runtime80.jsx)(
        "button",
        {
          type: "button",
          className: "text-text-muted dark:text-dark-text-muted hover:bg-surface-200 dark:hover:bg-dark-surface-300 inline-flex shrink-0 items-center justify-center rounded p-0.5",
          onClick: handleChevronClick,
          "aria-expanded": expanded,
          "aria-label": expanded ? "Collapse" : "Expand",
          children: /* @__PURE__ */ (0, import_jsx_runtime80.jsx)(
            import_lucide_react17.ChevronRight,
            {
              className: cn(
                "h-3.5 w-3.5 transition-transform",
                expanded && "rotate-90"
              )
            }
          )
        }
      ),
      /* @__PURE__ */ (0, import_jsx_runtime80.jsxs)(
        "button",
        {
          type: "button",
          onClick: () => onNodeSelect?.(node),
          className: "flex min-w-0 flex-1 items-center gap-1.5 rounded py-0.5 text-left hover:bg-transparent",
          children: [
            expanded ? /* @__PURE__ */ (0, import_jsx_runtime80.jsx)(import_lucide_react17.FolderOpen, { className: "h-4 w-4 shrink-0 text-warning dark:text-dark-warning" }) : /* @__PURE__ */ (0, import_jsx_runtime80.jsx)(import_lucide_react17.Folder, { className: "h-4 w-4 shrink-0 text-warning dark:text-dark-warning" }),
            /* @__PURE__ */ (0, import_jsx_runtime80.jsx)("span", { className: "truncate font-mono text-xs", children: node.name })
          ]
        }
      )
    ] }) : /* @__PURE__ */ (0, import_jsx_runtime80.jsxs)(
      "button",
      {
        type: "button",
        onClick: handleRowClick,
        onKeyDown: handleKeyDown,
        className: rowShellClass,
        style: { paddingLeft: depth * indent + 8 },
        children: [
          node.isDirectory ? /* @__PURE__ */ (0, import_jsx_runtime80.jsxs)(import_jsx_runtime80.Fragment, { children: [
            /* @__PURE__ */ (0, import_jsx_runtime80.jsx)(
              import_lucide_react17.ChevronRight,
              {
                className: cn(
                  "h-3.5 w-3.5 shrink-0 transition-transform",
                  "text-text-muted dark:text-dark-text-muted",
                  expanded && "rotate-90"
                )
              }
            ),
            expanded ? /* @__PURE__ */ (0, import_jsx_runtime80.jsx)(import_lucide_react17.FolderOpen, { className: "h-4 w-4 shrink-0 text-warning dark:text-dark-warning" }) : /* @__PURE__ */ (0, import_jsx_runtime80.jsx)(import_lucide_react17.Folder, { className: "h-4 w-4 shrink-0 text-warning dark:text-dark-warning" })
          ] }) : /* @__PURE__ */ (0, import_jsx_runtime80.jsxs)(import_jsx_runtime80.Fragment, { children: [
            /* @__PURE__ */ (0, import_jsx_runtime80.jsx)("span", { className: "w-3.5 shrink-0" }),
            /* @__PURE__ */ (0, import_jsx_runtime80.jsx)(import_lucide_react17.File, { className: "h-4 w-4 shrink-0 text-text-muted dark:text-dark-text-muted" })
          ] }),
          /* @__PURE__ */ (0, import_jsx_runtime80.jsx)("span", { className: "truncate font-mono text-xs", children: node.name })
        ]
      }
    ),
    node.isDirectory && expanded && node.children && /* @__PURE__ */ (0, import_jsx_runtime80.jsx)("div", { role: "group", children: node.children.map((child) => /* @__PURE__ */ (0, import_jsx_runtime80.jsx)(
      FileTreeItem,
      {
        node: child,
        depth: depth + 1,
        indent,
        selectedId,
        onNodeSelect,
        directoryClickMode,
        defaultExpanded
      },
      child.id
    )) })
  ] });
}

// src/components/ToolCallCard.tsx
var import_react40 = require("react");
var import_lucide_react18 = require("lucide-react");
var import_jsx_runtime81 = require("react/jsx-runtime");
var statusBadge = {
  pending: { variant: "secondary", label: "Pending" },
  running: { variant: "primary", label: "Running" },
  success: { variant: "success", label: "Done" },
  error: { variant: "error", label: "Error" }
};
var ToolCallCard = (0, import_react40.forwardRef)(
  function ToolCallCard2({
    className,
    tool,
    title,
    status = "success",
    icon,
    href,
    detail,
    duration,
    ...props
  }, ref) {
    const [open, setOpen] = (0, import_react40.useState)(false);
    const badge = statusBadge[status];
    return /* @__PURE__ */ (0, import_jsx_runtime81.jsxs)(
      "div",
      {
        ref,
        className: cn(
          "rounded-lg border",
          "border-surface-200 dark:border-dark-surface-500",
          "bg-surface-50 dark:bg-dark-surface-200",
          "text-text dark:text-dark-text",
          "transition-colors",
          className
        ),
        ...props,
        children: [
          /* @__PURE__ */ (0, import_jsx_runtime81.jsxs)("div", { className: "flex items-center gap-2 px-3 py-2", children: [
            icon && /* @__PURE__ */ (0, import_jsx_runtime81.jsx)("span", { className: "text-text-muted dark:text-dark-text-muted shrink-0", children: icon }),
            /* @__PURE__ */ (0, import_jsx_runtime81.jsx)(
              Span,
              {
                variant: "muted",
                className: "shrink-0 font-mono text-xs",
                children: tool
              }
            ),
            /* @__PURE__ */ (0, import_jsx_runtime81.jsx)(Span, { className: "min-w-0 flex-1 truncate text-sm font-medium", children: title }),
            /* @__PURE__ */ (0, import_jsx_runtime81.jsx)(Badge, { variant: badge.variant, size: "sm", children: badge.label }),
            duration && /* @__PURE__ */ (0, import_jsx_runtime81.jsx)(Span, { variant: "muted", className: "shrink-0 text-xs tabular-nums", children: duration }),
            href && /* @__PURE__ */ (0, import_jsx_runtime81.jsx)(
              "a",
              {
                href,
                target: "_blank",
                rel: "noopener noreferrer",
                className: "text-primary dark:text-dark-primary hover:text-primary-600 dark:hover:text-dark-primary-400 shrink-0",
                "aria-label": "Open in external tool",
                children: /* @__PURE__ */ (0, import_jsx_runtime81.jsx)(import_lucide_react18.ExternalLink, { className: "h-3.5 w-3.5" })
              }
            ),
            detail && /* @__PURE__ */ (0, import_jsx_runtime81.jsx)(
              "button",
              {
                type: "button",
                onClick: () => setOpen((v) => !v),
                className: cn(
                  "shrink-0 rounded p-0.5",
                  "text-text-muted dark:text-dark-text-muted",
                  "hover:bg-surface-100 dark:hover:bg-dark-surface-300"
                ),
                "aria-expanded": open,
                "aria-label": "Toggle detail",
                children: /* @__PURE__ */ (0, import_jsx_runtime81.jsx)(
                  import_lucide_react18.ChevronDown,
                  {
                    className: cn(
                      "h-4 w-4 transition-transform",
                      open && "rotate-180"
                    )
                  }
                )
              }
            )
          ] }),
          detail && open && /* @__PURE__ */ (0, import_jsx_runtime81.jsx)(
            "div",
            {
              className: cn(
                "border-t px-3 py-2",
                "border-surface-200 dark:border-dark-surface-500",
                "bg-surface-100 dark:bg-dark-surface-300",
                "overflow-auto text-xs font-mono",
                "text-text-muted dark:text-dark-text-muted",
                "max-h-60"
              ),
              children: detail
            }
          )
        ]
      }
    );
  }
);

// src/components/NavItem.tsx
var import_jsx_runtime82 = require("react/jsx-runtime");
function NavItem({
  isActive = false,
  icon,
  children,
  className,
  onClick,
  as: As,
  href,
  to
}) {
  const Tag = As ?? (href ? "a" : "div");
  return /* @__PURE__ */ (0, import_jsx_runtime82.jsxs)(
    Tag,
    {
      href,
      to,
      onClick,
      className: cn(
        "flex items-center gap-3 rounded-lg px-4 py-2.5 transition-colors",
        isActive ? "bg-primary-50/50 dark:bg-dark-primary-900/30 text-primary-700 dark:text-dark-primary-400 font-medium" : "text-text dark:text-dark-text hover:bg-surface-100 dark:hover:bg-dark-surface-100",
        className
      ),
      children: [
        icon && /* @__PURE__ */ (0, import_jsx_runtime82.jsx)("span", { className: "text-primary dark:text-dark-primary shrink-0", children: icon }),
        /* @__PURE__ */ (0, import_jsx_runtime82.jsx)("span", { className: "truncate", children })
      ]
    }
  );
}

// src/components/InlineAttachmentRenderer.tsx
var import_lucide_react19 = require("lucide-react");
var import_jsx_runtime83 = require("react/jsx-runtime");
function FileViewAttachment({
  attachment
}) {
  const lineCount = attachment.text.split("\n").length;
  return /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(
    Collapsible,
    {
      title: /* @__PURE__ */ (0, import_jsx_runtime83.jsxs)("span", { className: "inline-flex items-center gap-1.5 text-xs", children: [
        /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(import_lucide_react19.FileText, { className: "h-3.5 w-3.5" }),
        /* @__PURE__ */ (0, import_jsx_runtime83.jsx)("span", { className: "font-mono", children: attachment.path }),
        /* @__PURE__ */ (0, import_jsx_runtime83.jsxs)(Span, { variant: "muted", className: "text-[10px]", children: [
          lineCount,
          " ",
          lineCount === 1 ? "line" : "lines",
          attachment.truncated ? " \xB7 truncated" : ""
        ] })
      ] }),
      defaultExpanded: false,
      className: "border-surface-300 dark:border-dark-surface-400 bg-surface-100 dark:bg-dark-surface-200 mt-2 rounded border",
      children: /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(CodeBlock, { className: "px-3 py-2 leading-relaxed", children: attachment.text })
    }
  );
}
function TerminalExcerptAttachment({
  attachment
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(
    Collapsible,
    {
      title: /* @__PURE__ */ (0, import_jsx_runtime83.jsxs)("span", { className: "inline-flex items-center gap-1.5 text-xs", children: [
        /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(import_lucide_react19.TerminalSquare, { className: "h-3.5 w-3.5" }),
        /* @__PURE__ */ (0, import_jsx_runtime83.jsx)("span", { children: "Terminal output" }),
        attachment.command && /* @__PURE__ */ (0, import_jsx_runtime83.jsxs)(Span, { variant: "muted", className: "font-mono text-[10px]", children: [
          "$ ",
          attachment.command
        ] })
      ] }),
      defaultExpanded: false,
      className: "border-surface-300 dark:border-dark-surface-400 bg-surface-100 dark:bg-dark-surface-200 mt-2 rounded border",
      children: /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(CodeBlock, { className: "px-3 py-2 leading-relaxed", children: attachment.output })
    }
  );
}
function PlanSummaryAttachment({
  attachment
}) {
  const statusColor = attachment.status === "completed" ? "text-success" : attachment.status === "failed" ? "text-error" : "text-text-muted";
  return /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(
    Collapsible,
    {
      title: /* @__PURE__ */ (0, import_jsx_runtime83.jsxs)("span", { className: "inline-flex items-center gap-1.5 text-xs", children: [
        /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(import_lucide_react19.ListChecks, { className: "h-3.5 w-3.5" }),
        /* @__PURE__ */ (0, import_jsx_runtime83.jsxs)("span", { children: [
          "Plan step ",
          attachment.ordinal
        ] }),
        /* @__PURE__ */ (0, import_jsx_runtime83.jsxs)(Span, { variant: "muted", className: `text-[10px] ${statusColor}`, children: [
          "\xB7 ",
          attachment.status,
          attachment.failureClass ? ` (${attachment.failureClass})` : ""
        ] })
      ] }),
      defaultExpanded: false,
      className: "border-surface-300 dark:border-dark-surface-400 bg-surface-100 dark:bg-dark-surface-200 mt-2 rounded border",
      children: /* @__PURE__ */ (0, import_jsx_runtime83.jsxs)("div", { className: "space-y-1.5 px-3 py-2 text-xs", children: [
        /* @__PURE__ */ (0, import_jsx_runtime83.jsxs)("div", { children: [
          /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(Span, { variant: "muted", className: "text-[10px]", children: "Description" }),
          /* @__PURE__ */ (0, import_jsx_runtime83.jsx)("div", { className: "text-text dark:text-dark-text mt-0.5", children: attachment.description })
        ] }),
        attachment.summary && /* @__PURE__ */ (0, import_jsx_runtime83.jsxs)("div", { children: [
          /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(Span, { variant: "muted", className: "text-[10px]", children: "Summary" }),
          /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(CodeBlock, { className: "mt-0.5 text-[11px] whitespace-pre-wrap", children: attachment.summary })
        ] })
      ] })
    }
  );
}
function DAGAttachment({
  attachment
}) {
  return /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(
    Collapsible,
    {
      title: /* @__PURE__ */ (0, import_jsx_runtime83.jsxs)("span", { className: "inline-flex items-center gap-1.5 text-xs", children: [
        /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(import_lucide_react19.Workflow, { className: "h-3.5 w-3.5" }),
        /* @__PURE__ */ (0, import_jsx_runtime83.jsx)("span", { children: attachment.description ?? "Compiled chain DAG" })
      ] }),
      defaultExpanded: false,
      className: "border-surface-300 dark:border-dark-surface-400 bg-surface-100 dark:bg-dark-surface-200 mt-2 rounded border",
      children: /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(CodeBlock, { className: "px-3 py-2 text-[11px] leading-relaxed", children: attachment.chainJSON })
    }
  );
}
function StateUnitAttachment({
  attachment
}) {
  const data = attachment.data == null ? null : typeof attachment.data === "string" ? attachment.data : JSON.stringify(attachment.data, null, 2);
  return /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(
    Collapsible,
    {
      title: /* @__PURE__ */ (0, import_jsx_runtime83.jsxs)("span", { className: "inline-flex items-center gap-1.5 text-xs", children: [
        /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(import_lucide_react19.Database, { className: "h-3.5 w-3.5" }),
        /* @__PURE__ */ (0, import_jsx_runtime83.jsx)("span", { children: "Captured state" }),
        /* @__PURE__ */ (0, import_jsx_runtime83.jsxs)(Span, { variant: "muted", className: "text-[10px]", children: [
          "\xB7 ",
          attachment.name
        ] })
      ] }),
      defaultExpanded: false,
      className: "border-surface-300 dark:border-dark-surface-400 bg-surface-100 dark:bg-dark-surface-200 mt-2 rounded border",
      children: /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(CodeBlock, { className: "px-3 py-2 text-[11px] leading-relaxed", children: data ?? "(no data)" })
    }
  );
}
function InlineAttachmentRenderer({ attachment }) {
  switch (attachment.kind) {
    case "file_view":
      return /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(FileViewAttachment, { attachment });
    case "terminal_excerpt":
      return /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(TerminalExcerptAttachment, { attachment });
    case "plan_summary":
      return /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(PlanSummaryAttachment, { attachment });
    case "dag":
      return /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(DAGAttachment, { attachment });
    case "state_unit":
      return /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(StateUnitAttachment, { attachment });
    default: {
      const exhaustive = attachment;
      void exhaustive;
      return null;
    }
  }
}
function InlineAttachments({ attachments }) {
  if (!attachments || attachments.length === 0) return null;
  return /* @__PURE__ */ (0, import_jsx_runtime83.jsx)("div", { className: "mt-1 space-y-2", children: attachments.map((a, i) => /* @__PURE__ */ (0, import_jsx_runtime83.jsx)(InlineAttachmentRenderer, { attachment: a }, i)) });
}

// src/components/ErrorBoundary.tsx
var import_react41 = require("react");
var import_jsx_runtime84 = require("react/jsx-runtime");
var ErrorBoundary = class extends import_react41.Component {
  state = { error: null };
  static getDerivedStateFromError(error) {
    return { error };
  }
  componentDidCatch(error, info) {
    console.error("ErrorBoundary caught:", error, info);
  }
  reset = () => this.setState({ error: null });
  render() {
    if (this.state.error) {
      const { fallback } = this.props;
      if (typeof fallback === "function") {
        return fallback(this.state.error, this.reset);
      }
      return fallback ?? /* @__PURE__ */ (0, import_jsx_runtime84.jsx)("div", { className: "flex min-h-screen items-center justify-center", children: /* @__PURE__ */ (0, import_jsx_runtime84.jsxs)("div", { className: "text-center space-y-4", children: [
        /* @__PURE__ */ (0, import_jsx_runtime84.jsx)(H1, { children: "Something went wrong" }),
        /* @__PURE__ */ (0, import_jsx_runtime84.jsx)(P, { variant: "muted", children: this.state.error.message }),
        /* @__PURE__ */ (0, import_jsx_runtime84.jsx)(Button, { variant: "primary", onClick: this.reset, children: "Try again" })
      ] }) });
    }
    return this.props.children;
  }
};
// Annotate the CommonJS export names for ESM import in node:
0 && (module.exports = {
  Accordion,
  AddNodeButton,
  ApprovalCard,
  Badge,
  Blockquote,
  Button,
  ButtonGroup,
  Card,
  ChatComposer,
  ChatDateSeparator,
  ChatMessage,
  ChatProcessingBar,
  ChatScrollToLatest,
  ChatStreamThinkingBox,
  ChatStreamingCaret,
  ChatThread,
  ChatThreadSkeleton,
  ChatTranscriptStreamingPlaceholder,
  ChatTypingIndicator,
  Checkbox,
  CodeBlock,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  CommandPanel,
  Container,
  DEFAULT_COMPOSER_SOFT_MAX,
  DetailsPanel,
  Dialog,
  DiffView,
  DragDropContextProvider,
  Draggable,
  Dropdown,
  Droppable,
  EmptyState,
  ErrorBoundary,
  ErrorState,
  ExecutionTimeline,
  FileTree,
  Fill,
  Form,
  FormField,
  GridLayout,
  H1,
  H2,
  H3,
  Inbox,
  InlineAttachmentRenderer,
  InlineAttachments,
  InlineNotice,
  Input,
  InsetPanel,
  InsetPanelBody,
  InsetPanelHeader,
  JsonEditor,
  KeyValue,
  Label,
  LayoutControls,
  LoadingState,
  MonoLogList,
  MonoLogListItem,
  NavItem,
  NumberInput,
  P,
  Page,
  Pagination,
  Panel,
  PasswordInput,
  ProgressBar,
  ResizablePanel,
  ResizablePanelGroup,
  ResizablePanelHandle,
  ResourceCard,
  SearchBar,
  Section,
  Select,
  SelectOption,
  SidePanelBody,
  SidePanelColumn,
  SidePanelHeader,
  SidePanelRailButton,
  SidebarToggle,
  Skeleton,
  Small,
  Span,
  Spinner,
  StateVisualizer,
  TabPanel,
  TabPanels,
  TabTrigger,
  TabbedForm,
  TabbedPage,
  Table,
  TableCell,
  TableRow,
  Tabs,
  TaskEventFeed,
  TerminalOutput,
  Textarea,
  Toast,
  ToolCallCard,
  Toolbar,
  ToolbarActions,
  ToolbarItem,
  ToolbarSection,
  Tooltip,
  TranscriptEmbedCard,
  UserMenu,
  Wizard,
  WizardStep,
  WorkflowEdge,
  WorkflowNode,
  WorkflowVisualizer,
  calculateLayout,
  chatTranscriptMarkdownComponents,
  cn,
  copyTextToClipboard,
  getConnectorPath,
  isComposerCharCountWarning,
  isOverComposerSoftMax,
  useChatScroll,
  useCollapsibleContext,
  useDragDropContext
});
