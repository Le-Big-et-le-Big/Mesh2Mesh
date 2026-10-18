import { cn } from "@/lib/utils";

export function LogoMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" aria-hidden="true" className={cn("size-7", className)}>
      <rect width="32" height="32" rx="8" className="fill-primary" />
      <g className="stroke-primary-foreground" strokeWidth="2" strokeLinecap="round" opacity="0.55">
        <path d="M10 21.5 16 10.5 22 21.5Z" fill="none" />
      </g>
      <g className="fill-primary-foreground">
        <circle cx="16" cy="10.5" r="3" />
        <circle cx="10" cy="21.5" r="3" />
        <circle cx="22" cy="21.5" r="3" />
      </g>
    </svg>
  );
}

export function Logo({ className }: { className?: string }) {
  return (
    <span className={cn("inline-flex items-center gap-2.5 font-semibold tracking-tight text-foreground", className)}>
      <LogoMark />
      <span className="text-[17px]">Mesh2Mesh</span>
    </span>
  );
}
