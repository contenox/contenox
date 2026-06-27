import { Button } from "../Button";
import { Plus } from "lucide-react";
import React from "react";

interface AddNodeButtonProps {
  x: number;
  y: number;
  onClick: () => void;
  ariaLabel?: string;
  className?: string;
}

export const AddNodeButton: React.FC<AddNodeButtonProps> = ({
  x,
  y,
  onClick,
  ariaLabel = "Add workflow task",
  className,
}) => {
  return (
    <g
      transform={`translate(${x - 16}, ${y - 16})`}
      className={`cursor-pointer ${className}`}
    >
      <title>{ariaLabel}</title>
      <circle
        cx="16"
        cy="16"
        r="16"
        className="fill-primary-500 dark:fill-dark-primary-500 hover:fill-primary-600 dark:hover:fill-dark-primary-600 transition-colors duration-200"
      />
      <foreignObject width="32" height="32" x="0" y="0">
        <div className="flex h-8 w-8 items-center justify-center">
          <Button
            size="icon"
            variant="ghost"
            className="h-8 w-8 text-text-inverted hover:bg-primary-600 hover:text-text-inverted dark:text-dark-text-inverted dark:hover:bg-dark-primary-600 dark:hover:text-dark-text-inverted"
            onClick={onClick}
            aria-label={ariaLabel}
          >
            <Plus className="h-4 w-4" aria-hidden />
          </Button>
        </div>
      </foreignObject>
    </g>
  );
};

export default AddNodeButton;
