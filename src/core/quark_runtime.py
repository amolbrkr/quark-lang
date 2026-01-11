"""
Quark Runtime Library

Provides built-in functions and utilities for generated Python code.
"""


# Re-export Python builtins that Quark uses directly
from builtins import print, len, range, str, int, float, bool, list, dict


# Additional runtime utilities

def pipe(value, func):
    """
    Implement pipe operator: value | func
    Passes value as first argument to func.
    """
    if callable(func):
        return func(value)
    else:
        raise TypeError(f"Cannot pipe to non-callable: {func}")


def quark_range(start, end):
    """
    Quark range operator: start..end
    Creates a range from start to end (inclusive).
    """
    return range(start, end + 1)


# Pattern matching helpers (for future use)

def match_pattern(value, pattern):
    """
    Check if value matches pattern.
    Used in when statements.
    """
    if pattern == '_':
        return True
    return value == pattern


# Type conversion utilities

def to_number(value):
    """Convert value to number (int or float)"""
    try:
        if isinstance(value, (int, float)):
            return value
        if isinstance(value, str):
            if '.' in value:
                return float(value)
            return int(value)
        return int(value)
    except (ValueError, TypeError):
        raise TypeError(f"Cannot convert {value} to number")


def to_string(value):
    """Convert value to string"""
    return str(value)


def to_bool(value):
    """Convert value to boolean"""
    return bool(value)


# Collection utilities

def quark_list(*args):
    """Create list from arguments"""
    return list(args)


def quark_dict(**kwargs):
    """Create dictionary from keyword arguments"""
    return dict(kwargs)


# Math utilities (if needed later)

def power(base, exp):
    """Exponentiation: base ** exp"""
    return base ** exp


def mod(a, b):
    """Modulo: a % b"""
    return a % b


# String utilities

def concat(a, b):
    """String concatenation"""
    return str(a) + str(b)


# Comparison utilities

def equals(a, b):
    """Equality comparison"""
    return a == b


def not_equals(a, b):
    """Inequality comparison"""
    return a != b


def less_than(a, b):
    """Less than comparison"""
    return a < b


def less_than_or_equal(a, b):
    """Less than or equal comparison"""
    return a <= b


def greater_than(a, b):
    """Greater than comparison"""
    return a > b


def greater_than_or_equal(a, b):
    """Greater than or equal comparison"""
    return a >= b


# Logical operators

def logical_and(a, b):
    """Logical AND"""
    return a and b


def logical_or(a, b):
    """Logical OR"""
    return a or b


def logical_not(a):
    """Logical NOT"""
    return not a


# Export all runtime functions
__all__ = [
    # Python builtins
    'print', 'len', 'range', 'str', 'int', 'float', 'bool', 'list', 'dict',

    # Quark runtime
    'pipe', 'quark_range', 'match_pattern',
    'to_number', 'to_string', 'to_bool',
    'quark_list', 'quark_dict',
    'power', 'mod', 'concat',
    'equals', 'not_equals',
    'less_than', 'less_than_or_equal',
    'greater_than', 'greater_than_or_equal',
    'logical_and', 'logical_or', 'logical_not'
]
