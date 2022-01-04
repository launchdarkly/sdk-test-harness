# Test data files

The SDK test harness is not purely data-driven as some test harnesses are; most of the tests are written in Go code, rather than declared in data. However, for some very repetitive tests it is convenient to provide parameters in data files.

Files should be grouped in subdirectories if they are for the same test and use the same schema.

## File format

The format of these files varies according to the test code that is using them, but there are some things in common.

Files may be either JSON or YAML. YAML has several advantages:

* It's often more concise.
* Comments are allowed.
* Repetitive property lists can be simplified using anchor references.

There are two convenience features that the test harness supports in both JSON and YAML. First, you can define constants of any JSON type, in a top-level property called `constants`. Any occurrence of `<SOME_CONSTANT_NAME>` elsewhere in the file will be replaced by the value of that constant.

Second, you can define sets of parameters which will cause the file to be in effect duplicated for each set of parameters. The top-level property `parameters`, if present, should be an array of JSON objects; each object is treated the same as it would be for `constants`.

For example, the following YAML file...

```yaml
---
constants:
  COLOR: green

parameters:
  - DAY: Monday
    FOOD: eggs
  - DAY: Tuesday
    FOOD: beans

some-test-property:
  message: "On <DAY> we eat <COLOR> <FOOD>"
```

...would be treated as if it were two files:

```yaml
---
some-test-property:
  message: "On Monday we eat green eggs"
```

```yaml
---
some-test-property:
  message: "On Tuesday we eat green beans"
```

You can also define a list of parameter lists, which will expand into every permutation. For instance, this...

```yaml
parameters:
  -
    - DAY: Monday
    - DAY: Tuesday
  -
    - FOOD: eggs
    - FOOD: beans
```

...is equivalent to:

```yaml
parameters:
  - DAY: Monday
    FOOD: eggs
  - DAY: Tuesday
    FOOD: eggs
  - DAY: Monday
    FOOD: beans
  - DAY: Tuesday
    FOOD: beans
```
