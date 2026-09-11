class Outer:
    class Inner:
        def method_b(self):
            return 2

        def method_a(self):
            return 10

        def method_c(self):
            return 3

    def outer_method(self):
        return self.Inner()
