# Automapper Generator
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.21-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

A CLI utility that generates reflection-free Go code for mapping structs with type conversion support.


> **⚠️ Development Status**: This project is in early development and serves primarily as a proof of concept. Use in production at your own risk.

## Features

- 🚀 **Zero Reflection**: Generates type-safe code at compile time
- 🔄 **Type Conversion**: Built-in converter system with custom converters
- 📦 **External Packages**: Map from structs in other packages
- 🏷️ **Flexible Mapping**: Tag-based field mapping and transformations
- 🎯 **Type Safety**: Compile-time type checking with generics
- ⚡ **Performance**: Direct field assignments, no runtime overhead

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Usage](#usage)
- [Examples](#examples)
- [Development](#development)
- [How It Works](#how-it-works)
- [Contributing](#contributing)

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/yourusername/automapper-gen.git
cd automapper-gen

# Build and install
make install

# Or build only
make build
```

### Using Go Install

```bash
go install github.com/yourusername/automapper-gen/cmd/automapper-gen@latest
```

## Quick Start

### 1. Create Configuration

Create an `automapper.json` file in your DTO package directory:

```json
{
  "package": "dtos",
  "output": "automappers.go",
  "fieldNameTransform": "snake_to_camel",
  "generateInit": true,
  "defaultConverters": [
    {
      "from": "time.Time",
      "to": "string",
      "name": "TimeToString",
      "function": "TimeToJSString"
    }
  ],
  "externalPackages": [
    {
      "alias": "db",
      "importPath": "github.com/yourorg/yourproject/internal/db",
      "localPath": "../db"
    }
  ]
}
```

### 2. Define Your Structs

**Database Model** (`internal/db/models.go`):
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

**DTO Model** (`internal/dtos/user.go`):
```go
package dtos

// automapper:from=db.UserDB
type UserDTO struct {
    ID        int64
    Username  string
    Email     string
    CreatedAt string `automapper:"converter=TimeToString"`
    UpdatedAt string `automapper:"converter=TimeToString"`
}
```

### 3. Generate Mappers

```bash
# From the dtos directory
automapper-gen .

# Or using make from project root
make generate
```

### 4. Use the Generated Code

```go
package main

import (
    "fmt"
    "time"
    "yourproject/internal/db"
    "yourproject/internal/dtos"
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

## Configuration

### Configuration File (`automapper.json`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `package` | string | Yes | Target package name |
| `output` | string | No | Output filename (default: "automappers.go") |
| `fieldNameTransform` | string | No | Field name transformation ("snake_to_camel", default) |
| `nilPointersForNull` | bool | No | Use nil pointers for null values |
| `generateInit` | bool | No | Generate init() function for converters |
| `defaultConverters` | array | No | Default converter registrations |
| `externalPackages` | array | No | External packages to parse |

### Default Converters

```json
{
  "defaultConverters": [
    {
      "from": "time.Time",
      "to": "string",
      "name": "TimeToString",
      "function": "TimeToJSString"
    }
  ]
}
```

### External Packages

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

## Usage

### Basic Mapping

```go
// automapper:from=SourceStruct
type TargetDTO struct {
    Field1 string
    Field2 int
}
```

### Multiple Source Structs

```go
// automapper:from=User,Profile
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
    CreatedAt string `automapper:"converter=TimeToString"`
}
```

#### Combined Tags
```go
type UserDTO struct {
    BirthDate string `automapper:"field=date_of_birth,converter=TimeToString"`
}
```

### Custom Converters

Register custom converters in your code:

```go
func init() {
    // Register a custom UUID to string converter
    dtos.RegisterConverter[uuid.UUID, string](
        "UUIDToString",
        func(u uuid.UUID) (string, error) {
            return u.String(), nil
        },
    )
}
```

Use in your DTOs:

```go
type UserDTO struct {
    ID string `automapper:"converter=UUIDToString"`
}
```

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

// automapper:from=db.User
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

// automapper:from=db.Product
type ProductDTO struct {
    Id        int64
    Name      string
    Price     float64
    CreatedAt string `automapper:"converter=TimeToString"`
}
```

### Example 3: Complex Mapping

Run the included example:

```bash
make example
```

This runs the code in `example/` directory which demonstrates:
- External package mapping
- Type conversions
- Field name transformations
- Custom field mappings

## Development

### Prerequisites

- Go 1.21 or higher
- Make (optional, but recommended)

### Building

```bash
# Build the binary
make build

# Install to GOPATH
make install

# Run tests
make test

# Run benchmarks
make bench

# Format code
make fmt

# Lint code (requires golangci-lint)
make lint

# Run all checks
make check
```

### Testing

```bash
# Run all tests with coverage
make test

# View coverage report
open coverage.html

# Run specific tests
go test -v ./internal/parser/
go test -v ./internal/generator/
```

### Project Commands

```bash
# Generate example mappers
make generate

# Run example
make example

# Clean build artifacts
make clean

# Create release (requires goreleaser)
make release
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

// Init function (if generateInit: true)
func init() {
    RegisterConverter[time.Time, string]("TimeToString", TimeToJSString)
}

// MapFrom methods
func (d *UserDTO) MapFromUserDB(src *db.UserDB) error {
    if src == nil {
        return errors.New("source is nil")
    }
    d.ID = src.ID
    d.Username = src.Username
    // ... field mappings
    return nil
}
```

## Acknowledgments

- [jennifer](https://github.com/dave/jennifer) - Go code generation library
- Inspired by AutoMapper in .NET
