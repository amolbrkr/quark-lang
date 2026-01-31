#pragma once

#include "ast.h"

class QuarkCodegen
{
public:
	QuarkCodegen() = default;

	void begin(const TreeNode& tree);
};