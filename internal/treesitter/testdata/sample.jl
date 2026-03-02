module Geometry

struct Point
    x::Float64
    y::Float64
end

mutable struct Counter
    count::Int
end

abstract type Shape end

const MAX_SIZE = 100

function distance(a::Point, b::Point)
    dx = a.x - b.x
    dy = a.y - b.y
    return sqrt(dx^2 + dy^2)
end

function identity()
    return nothing
end

area(r) = π * r^2

macro debug(ex)
    return esc(ex)
end

@debug println("test")

export distance, area

using LinearAlgebra

import Base: show

end
