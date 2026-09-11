package com.example;

import java.util.List;

public interface Repository {
    Object findById(String id);

    List<Object> findAll();
}
