use crate::ast::AstNode;
use std::fmt::Write;

pub struct Visualizer {
    dot_output: String,
    node_counter: usize,
}

impl Visualizer {
    pub fn new() -> Self {
        Self {
            dot_output: String::new(),
            node_counter: 0,
        }
    }

    pub fn visualize(&mut self, root: &AstNode) -> String {
        self.dot_output.clear();
        self.node_counter = 0;

        writeln!(&mut self.dot_output, "digraph AST {{").unwrap();
        writeln!(&mut self.dot_output, "    node [shape=box];").unwrap();
        writeln!(&mut self.dot_output, "    rankdir=TB;").unwrap();

        self.visit_node(root, None);

        writeln!(&mut self.dot_output, "}}").unwrap();

        self.dot_output.clone()
    }

    fn visit_node(&mut self, node: &AstNode, parent_id: Option<usize>) -> usize {
        let current_id = self.node_counter;
        self.node_counter += 1;

        // Create node label
        let label = self.create_label(node);
        let escaped_label = self.escape_label(&label);

        writeln!(
            &mut self.dot_output,
            "    node{} [label=\"{}\"];",
            current_id, escaped_label
        )
        .unwrap();

        // Create edge from parent
        if let Some(parent) = parent_id {
            writeln!(&mut self.dot_output, "    node{} -> node{};", parent, current_id).unwrap();
        }

        // Visit children
        for child in &node.children {
            self.visit_node(child, Some(current_id));
        }

        current_id
    }

    fn create_label(&self, node: &AstNode) -> String {
        let node_type_str = format!("{:?}", node.node_type);

        if let Some(ref token) = node.token {
            if !token.lexeme.is_empty() {
                format!("{}\\n'{}'", node_type_str, token.lexeme)
            } else {
                node_type_str
            }
        } else {
            node_type_str
        }
    }

    fn escape_label(&self, label: &str) -> String {
        label
            .replace('\\', "\\\\")
            .replace('"', "\\\"")
            .replace('\n', "\\n")
    }
}

impl Default for Visualizer {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ast::NodeType;

    #[test]
    fn test_simple_visualization() {
        let mut root = AstNode::new(NodeType::CompilationUnit, None);
        let child = AstNode::new(NodeType::Expression, None);
        root.add_child(child);

        let mut viz = Visualizer::new();
        let dot = viz.visualize(&root);

        assert!(dot.contains("digraph AST"));
        assert!(dot.contains("CompilationUnit"));
        assert!(dot.contains("Expression"));
    }
}
