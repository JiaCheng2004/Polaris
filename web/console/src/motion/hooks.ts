import { useRef, type RefObject } from "react";
import gsap from "gsap";
import { useGSAP } from "@gsap/react";

gsap.registerPlugin(useGSAP);

/**
 * Staggered entrance for elements marked [data-reveal] inside `scope`.
 *
 * Motion is an experience-first affordance: it runs in STANDARD mode only. Advanced
 * mode is information-first (first-principles, minimal effect), so `enabled` is false
 * there and the elements render immediately with no transform. Transform/opacity only
 * (compositor-friendly); also disabled under prefers-reduced-motion via matchMedia.
 */
export function useReveal(
  scope: RefObject<HTMLElement | null>,
  deps: unknown[] = [],
  enabled = true
) {
  useGSAP(
    () => {
      if (!enabled) return;
      const mm = gsap.matchMedia();
      mm.add(
        { animate: "(prefers-reduced-motion: no-preference)" },
        (ctx) => {
          if (!ctx.conditions?.animate) return;
          gsap.from("[data-reveal]", {
            opacity: 0,
            y: 12,
            duration: 0.5,
            ease: "power3.out",
            stagger: 0.045,
            clearProps: "transform,opacity",
          });
        },
        scope as unknown as Element
      );
      return () => mm.revert();
    },
    { scope: scope as RefObject<HTMLElement>, dependencies: [...deps, enabled], revertOnUpdate: true }
  );
}

/**
 * Tween a numeric text node from its previous value to `value`. Standard mode only
 * (`enabled`); in Advanced the exact value is written immediately with no animation.
 */
export function useCountUp(value: number, format: (n: number) => string, enabled = true) {
  const ref = useRef<HTMLSpanElement>(null);
  const prev = useRef(0);
  useGSAP(
    () => {
      const el = ref.current;
      if (!el) return;
      const from = prev.current;
      prev.current = value;
      if (
        !enabled ||
        window.matchMedia("(prefers-reduced-motion: reduce)").matches ||
        from === value
      ) {
        el.textContent = format(value);
        return;
      }
      const obj = { n: from };
      gsap.to(obj, {
        n: value,
        duration: 0.7,
        ease: "power2.out",
        onUpdate: () => {
          el.textContent = format(obj.n);
        },
      });
    },
    { dependencies: [value, enabled] }
  );
  return ref;
}
