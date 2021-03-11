#ifndef _TRACING_BPF_HELPERS_H
#define _TRACING_BPF_HELPERS_H

#ifdef asm_volatile_goto
#undef asm_volatile_goto
#endif
#define asm_volatile_goto(x...) asm volatile("invalid use of asm_volatile_goto")

#ifdef CONFIG_CC_HAS_ASM_INLINE
#undef CONFIG_CC_HAS_ASM_INLINE
#endif

// #ifdef asm_inline
// #undef asm_inline
// #endif
// #define asm_inline asm


#endif /* _TRACING_BPF_HELPERS_H */