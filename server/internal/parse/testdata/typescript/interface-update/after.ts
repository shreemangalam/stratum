interface User {
    id: string;
    name: string;
    email: string;
    role: "admin" | "user";
}

function getUser(id: string): User {
    return { id, name: "test", email: "test@example.com", role: "user" };
}

function formatUser(user: User): string {
    return `${user.name} (${user.role}) <${user.email}>`;
}

function deleteUser(id: string): void {
    console.log(`Deleting user ${id}`);
}
