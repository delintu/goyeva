![Yeva](./ext/images/logo.png)

```js
struct Vector {
  .new(x, y) {
    x ??= 0
    y ??= 0
    return struct (Vector) { .x, .y }
  },
  ->add(other) {
    return Vector.new(
      this.x + other.x,
      this.y + other.y,
    )
  },
}

var vec1 = Vector.new(1, 2)
var vec2 = Vector.new(3, 4)
var vec3 = vec1->add(vec2)

print("x =", vec3.x)
print("y =", vec3.y)
```
