class Outer:
    class Inner:
        def method_a(self):
            return 1

        def method_b(self):
            return 2

    def outer_method(self):
        return self.Inner()
