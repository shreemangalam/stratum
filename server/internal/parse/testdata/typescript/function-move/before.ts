export function validate(input: string): boolean {
    return input.length > 0;
}

export function sanitize(input: string): string {
    return input.trim();
}

export function transform(input: string): string {
    return input.toUpperCase();
}
