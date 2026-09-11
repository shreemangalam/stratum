class UserService:
    def get_user(self, user_id):
        return self.db.find(user_id)

    def delete_user(self, user_id):
        self.db.remove(user_id)
