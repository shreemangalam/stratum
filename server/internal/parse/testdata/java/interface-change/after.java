package com.example;

import java.util.List;
import java.util.Optional;

public interface Repository {
    Optional<Object> findById(String id);

    List<Object> findAll();

    void deleteById(String id);
}
