import { useRef, useState } from "react";
import { CheckIcon, CopyIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

// The clipboard API needs https or localhost; onFail can select the text instead.
export function useCopy(onFail?: () => void) {
  const [copied, setCopied] = useState(false);
  const copy = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      onFail?.();
    }
  };
  return [copied, copy] as const;
}

export function CopyValue({ value, display, className }: { value: string; display?: string; className?: string }) {
  const [copied, copy] = useCopy();
  return (
    <button
      type="button"
      onClick={() => copy(value)}
      title="Copy"
      className={cn(
        "group inline-flex items-center gap-1.5 rounded-md px-1.5 py-0.5 -mx-1.5 font-mono text-[13px] hover:bg-accent",
        className,
      )}
    >
      <span className="truncate">{display ?? value}</span>
      {copied ? (
        <CheckIcon className="size-3.5 text-success" />
      ) : (
        <CopyIcon className="size-3.5 text-muted-foreground opacity-0 group-hover:opacity-100" />
      )}
    </button>
  );
}

export function Command({ children, className }: { children: string; className?: string }) {
  const ref = useRef<HTMLElement>(null);
  const [copied, copy] = useCopy(() => ref.current && window.getSelection()?.selectAllChildren(ref.current));
  return (
    <div className={cn("flex items-center gap-2 rounded-lg border bg-muted py-1.5 pr-1.5 pl-3.5", className)}>
      <span className="select-none font-mono text-[13px] text-muted-foreground">$</span>
      <code ref={ref} className="flex-1 overflow-x-auto whitespace-pre py-1 text-[13px]">
        {children}
      </code>
      <Button type="button" variant="ghost" size="sm" onClick={() => copy(children)} className="shrink-0">
        {copied ? <CheckIcon className="text-success" /> : <CopyIcon />}
        {copied ? "Copied" : "Copy"}
      </Button>
    </div>
  );
}
