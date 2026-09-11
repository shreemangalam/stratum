export function sanitize(input: string): string {
    return input.trim();
}

export function transform(input: string): string {
    return input.toLowerCase();
}

export function validate(input: string): boolean {
    return input.length > 0;
}
