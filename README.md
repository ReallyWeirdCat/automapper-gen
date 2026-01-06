# Automapper Generator
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.21-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

A CLI utility that generates reflection-free Go code for mapping structs with type conversion support.

> **⚠️ Development Status**: This project is in early development. There are yet no integrated tests, and many things may change in the near future. Use in production at your own risk (and if you do, please tell me if you like it).

## Features

- 🚀 **Zero Reflection**: Generates type-safe code at compile time
- 🔄 **Type Conversion**: Built-in converter system with custom converters
- 📦 **Remote Packages**: Map from structs in any Go module (local or remote)
- 🌐 **Module Cache Support**: Automatically loads types from Go's module cache
- 🏷️ **Flexible Mapping**: Tag-based field mapping and transformations
- 🎯 **Type Safety**: Compile-time type checking with generics
- ⚡ **Performance**: Direct field assignments, no runtime overhead

[![Marilyn Manson - No Reflection](https://img.youtube.com/vi/DOj3wDlr_BM/0.jpg)](https://www.youtube.com/watch?v=DOj3wDlr_BM)

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Usage](#usage)
  - [Remote Modules](#remote-modules)
  - [Basic Mapping](#basic-mapping)
  - [Multiple Source Structs](#multiple-source-structs)
  - [Field Tags](#field-tags)
  - [Custom Converters](#custom-converters)
- [Examples](#examples)
- [How It Works](#how-it-works)

## Installation

### From Source

```bash
# Clone the repository
git clone https://git.weirdcat.su/weirdcat/automapper-gen.git
cd automapper-gen

# Build and install
make install

# Or build only
make build
```

### Using Go Install

```bash
go install git.weirdcat.su/weirdcat/automapper-gen/cmd/automapper-gen@latest
```

## Quick Start

### 1. Create Configuration

Create an `automapper.json` file in your DTO package directory:

```json
{
  "package": "dtos",
  "output": "automappers.go",
  "externalPackages": [
    {
      "alias": "db",
      "importPath": "git.weirdcat.su/weirdcat/automapper-gen/example/db",
      "localPath": "../db"
    }
  ]
}
```

**Note**: External packages are loaded directly from Go's module cache. Use `localPath` for development to test local changes.

### 2. Define Your Structs

**Database Model** (`db/models.go`):
```go
package db

import "time"

type UserDB struct {
    ID        int64
    Username  string
    Email     string
    Password  string
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

**DTO Model** (`dtos/user.go`):
```go
package dtos

//automapper:from=db.UserDB
type UserDTO struct {
    ID        int64
    Username  string
    Email     string
    CreatedAt string `automapper:"converter=TimeToJSString"`
    UpdatedAt string `automapper:"converter=TimeToJSString"`
}
```

### 3. Generate Mappers

```bash
# From the dtos directory
automapper-gen .
```

### 4. Use the Generated Code

```go
package main

import (
    "fmt"
    "time"
    "yourproject/db"
    "yourproject/dtos"
)

func main() {
    // Source data
    user := &db.UserDB{
        ID:        1,
        Username:  "john_doe",
        Email:     "john@example.com",
        Password:  "hashed_password",
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }

    // Map to DTO
    dto := &dtos.UserDTO{}
    if err := dto.MapFromUserDB(user); err != nil {
        panic(err)
    }

    fmt.Printf("User: %+v\n", dto)
    // Output: User: {ID:1 Username:john_doe Email:john@example.com CreatedAt:2024-01-05T10:30:00Z UpdatedAt:2024-01-05T10:30:00Z}
}
```

**Note**: The `TimeToJSString` converter is built-in and automatically available for use in struct tags.

## Configuration

### Configuration File (`automapper.json`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `package` | string | Yes | Target package name |
| `output` | string | No | Output filename (default: "automappers.go") |
| `fieldNameTransform` | string | No | Field name transformation ("snake_to_camel", default) |
| `nilPointersForNull` | bool | No | Use nil pointers for null values |
| `externalPackages` | array | No | External packages to parse |

### External Packages

External packages are loaded directly from Go's module cache, making it easy to map from types in any Go module:

```json
{
  "externalPackages": [
    {
      "alias": "db",
      "importPath": "github.com/yourorg/project/db"
    },
    {
      "alias": "models",
      "importPath": "git.example.com/team/service/models"
    }
  ]
}
```

**Local Development**: If you're working on a module locally and want to use local changes:

```json
{
  "externalPackages": [
    {
      "alias": "db",
      "importPath": "github.com/yourorg/project/db",
      "localPath": "../db"
    }
  ]
}
```

The generator will try the local path first, then fall back to the module cache.

## Usage

### Remote Modules

One of the key features is the ability to map from types in any Go module, whether it's in your repository or a completely separate one:

#### Example: Mapping from a Different Repository

**Repository 1** (`git.weirdcat.su/test/prj1`):
```go
// bd/models.go
package bd

import "time"

type User struct {
    ID        int64
    Username  string
    CreatedAt time.Time
}
```

**Repository 2** (`git.weirdcat.su/test/prj2`):
```json
// dto/automapper.json
{
  "package": "dto",
  "output": "automappers.go",
  "externalPackages": [
    {
      "alias": "db",
      "importPath": "git.weirdcat.su/test/prj1/bd"
    }
  ]
}
```

```go
// dto/user.go
package dto

//automapper:from=db.User
type UserDTO struct {
    ID        int64
    Username  string
    CreatedAt string `automapper:"converter=TimeToJSString"`
}
```

**Prerequisites**: Make sure the external module is in your `go.mod`:
```bash
go get git.weirdcat.su/test/prj1
```

Then generate:
```bash
cd dto
automapper-gen .
```

The generator will load the `bd` package from your module cache and generate the appropriate mappers!

### Basic Mapping

```go
//automapper:from=SourceStruct
type TargetDTO struct {
    Field1 string
    Field2 int
}
```

### Multiple Source Structs

```go
//automapper:from=User,Profile
type UserProfileDTO struct {
    // Will generate MapFromUser and MapFromProfile
    Name  string
    Email string
}
```

### Field Tags

#### Skip Field
```go
type UserDTO struct {
    Password string `automapper:"-"`  // Will not be mapped
}
```

#### Custom Field Mapping
```go
type UserDTO struct {
    Name string `automapper:"field=full_name"`  // Maps from full_name
}
```

#### Field Converter
```go
type UserDTO struct {
    CreatedAt string `automapper:"converter=TimeToJSString"`
}
```

#### Combined Tags
```go
type UserDTO struct {
    BirthDate string `automapper:"field=date_of_birth,converter=TimeToJSString"`
}
```

### Custom Converters

Create and register custom converters in your code:

```go
package dtos

import (
    "fmt"
    "git.weirdcat.su/weirdcat/automapper-gen/example/types"
)

// Define your converter function
func StrRoleToEnum(role string) (types.Role, error) {
    switch role {
    case "admin":
        return types.RoleAdmin, nil
    case "user":
        return types.RoleUser, nil
    default:
        return types.RoleGuest, fmt.Errorf("unknown role: %s", role)
    }
}

func StrInterestsToEnums(interests []string) ([]types.Interest, error) {
    result := make([]types.Interest, len(interests))
    for i, interest := range interests {
        switch interest {
        case "coding":
            result[i] = types.InterestCoding
        case "music":
            result[i] = types.InterestMusic
        default:
            return nil, fmt.Errorf("unknown interest: %s", interest)
        }
    }
    return result, nil
}
```

The generated `automappers.go` will automatically include an `init()` function that registers these converters based on their function signatures.

Use in your DTOs:

```go
type UserDTO struct {
    Role      Role      `automapper:"converter=StrRoleToEnum"`
    Interests []Interest `automapper:"converter=StrInterestsToEnums"`
}
```

**Note**: Converter functions must follow the signature `func(T) (U, error)` and be in the same package as your DTOs.

## Examples

### Example 1: Basic User Mapping

**Source** (`db/user.go`):
```go
package db

type User struct {
    id       int64
    username string
    email    string
}
```

**DTO** (`dtos/user.go`):
```go
package dtos

//automapper:from=db.User
type UserDTO struct {
    Id       int64
    Username string
    Email    string
}
```

**Usage**:
```go
user := &db.User{id: 1, username: "john", email: "john@example.com"}
dto := &dtos.UserDTO{}
dto.MapFromUser(user)
```

### Example 2: With Type Conversion

**Source** (`db/product.go`):
```go
package db

import "time"

type Product struct {
    id         int64
    name       string
    price      float64
    created_at time.Time
}
```

**DTO** (`dtos/product.go`):
```go
package dtos

//automapper:from=db.Product
type ProductDTO struct {
    Id        int64
    Name      string
    Price     float64
    CreatedAt string `automapper:"converter=TimeToJSString"`
}
```

## How It Works

1. **Parse Configuration**: Reads `automapper.json` to understand the project setup
2. **Parse Source Code**: Uses Go's AST parser to analyze structs and fields
3. **Extract Annotations**: Finds `// automapper:from=...` comments on DTOs
4. **Match Fields**: Maps source fields to target fields using name transforms and tags
5. **Generate Code**: Uses [jennifer](https://github.com/dave/jennifer) to generate type-safe Go code
6. **Write Output**: Creates the mapper file with all MapFrom methods

### Generated Code Structure

```go
// Converter infrastructure
type Converter[From any, To any] func(From) (To, error)
var converters = make(map[string]interface{})

func RegisterConverter[From any, To any](name string, fn Converter[From, To])
func Convert[From any, To any](name string, value From) (To, error)

// Built-in converter (always available)
func TimeToJSString(t time.Time) (string, error) {
    return t.Format(time.RFC3339), nil
}

// MapFrom methods
func (d *UserDTO) MapFromUserDB(src *db.UserDB) error {
    if src == nil {
        return errors.New("source is nil")
    }
    d.ID = src.ID
    d.Username = src.Username
    // ... field mappings with converters
    return nil
}
```

## Built-in Converters

The following converters are automatically generated and available:

- `TimeToJSString`: Converts `time.Time` to RFC3339 string format

## Custom Converter Registration

Converter functions in your DTO package are automatically registered in the generated `init()` function if they follow the correct signature. The types are inferred from the function signature, so you don't need to specify them in configuration.

## Acknowledgments

- [jennifer](https://github.com/dave/jennifer) - Go code generation library
- Inspired by AutoMapper in .NET
