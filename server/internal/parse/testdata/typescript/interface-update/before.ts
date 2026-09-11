interface User {
    id: string;
    name: string;
    email: string;
}

function getUser(id: string): User {
    return { id, name: "test", email: "test@example.com" };
}

function formatUser(user: User): string {
    return `${user.name} <${user.email}>`;
}
