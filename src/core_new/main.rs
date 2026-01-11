use anyhow::{Context, Result};
use clap::{Parser, Subcommand};
use quark::{Lexer, Parser as QuarkParser, Visualizer};
use std::fs;
use std::path::PathBuf;
use std::process::Command;

#[derive(Parser)]
#[command(name = "quark")]
#[command(about = "Quark language compiler", long_about = None)]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Tokenize a source file and display tokens
    Lex {
        /// Input source file
        #[arg(value_name = "FILE")]
        file: PathBuf,

        /// Print detailed token information
        #[arg(short, long)]
        verbose: bool,
    },

    /// Parse a source file and display the AST
    Parse {
        /// Input source file
        #[arg(value_name = "FILE")]
        file: PathBuf,

        /// Print tree structure to console
        #[arg(short, long)]
        tree: bool,
    },

    /// Parse and visualize the AST as a PNG image
    Visualize {
        /// Input source file
        #[arg(value_name = "FILE")]
        file: PathBuf,

        /// Output DOT file (default: treeviz.dot)
        #[arg(short, long, default_value = "treeviz.dot")]
        dot_output: PathBuf,

        /// Output PNG file (default: treeviz.png)
        #[arg(short, long, default_value = "treeviz.png")]
        png_output: PathBuf,

        /// Skip PNG generation (only generate DOT file)
        #[arg(long)]
        no_png: bool,
    },

    /// Complete pipeline: lex -> parse -> visualize
    Run {
        /// Input source file
        #[arg(value_name = "FILE")]
        file: PathBuf,

        /// Output DOT file (default: treeviz.dot)
        #[arg(short, long, default_value = "treeviz.dot")]
        dot_output: PathBuf,

        /// Output PNG file (default: treeviz.png)
        #[arg(short, long, default_value = "treeviz.png")]
        png_output: PathBuf,
    },
}

fn main() -> Result<()> {
    let cli = Cli::parse();

    match cli.command {
        Commands::Lex { file, verbose } => lex_command(file, verbose),
        Commands::Parse { file, tree } => parse_command(file, tree),
        Commands::Visualize {
            file,
            dot_output,
            png_output,
            no_png,
        } => visualize_command(file, dot_output, png_output, no_png),
        Commands::Run {
            file,
            dot_output,
            png_output,
        } => run_command(file, dot_output, png_output),
    }
}

fn lex_command(file: PathBuf, verbose: bool) -> Result<()> {
    let source = fs::read_to_string(&file)
        .context(format!("Failed to read file: {}", file.display()))?;

    let mut lexer = Lexer::new(&source);
    let tokens = lexer.tokenize().context("Lexing failed")?;

    println!("=== Lexer Output ===");
    println!("Total tokens: {}\n", tokens.len());

    for (i, token) in tokens.iter().enumerate() {
        if verbose {
            println!("{}. {}", i, token);
        } else {
            println!("{:?}('{}')", token.token_type, token.lexeme);
        }
    }

    Ok(())
}

fn parse_command(file: PathBuf, tree: bool) -> Result<()> {
    let source = fs::read_to_string(&file)
        .context(format!("Failed to read file: {}", file.display()))?;

    let mut lexer = Lexer::new(&source);
    let tokens = lexer.tokenize().context("Lexing failed")?;

    let mut parser = QuarkParser::new(tokens);
    let ast = parser.parse().context("Parsing failed")?;

    println!("=== Parser Output ===");
    println!("AST generated successfully!\n");

    if tree {
        println!("AST Structure:");
        println!("{}", ast);
    } else {
        println!("Use --tree flag to display the AST structure");
    }

    Ok(())
}

fn visualize_command(
    file: PathBuf,
    dot_output: PathBuf,
    png_output: PathBuf,
    no_png: bool,
) -> Result<()> {
    let source = fs::read_to_string(&file)
        .context(format!("Failed to read file: {}", file.display()))?;

    let mut lexer = Lexer::new(&source);
    let tokens = lexer.tokenize().context("Lexing failed")?;

    let mut parser = QuarkParser::new(tokens);
    let ast = parser.parse().context("Parsing failed")?;

    let mut visualizer = Visualizer::new();
    let dot_content = visualizer.visualize(&ast);

    fs::write(&dot_output, &dot_content)
        .context(format!("Failed to write DOT file: {}", dot_output.display()))?;

    println!("✓ Generated DOT file: {}", dot_output.display());

    if !no_png {
        generate_png(&dot_output, &png_output)?;
    }

    Ok(())
}

fn run_command(file: PathBuf, dot_output: PathBuf, png_output: PathBuf) -> Result<()> {
    println!("=== Quark Compiler Pipeline ===\n");

    let source = fs::read_to_string(&file)
        .context(format!("Failed to read file: {}", file.display()))?;

    // Lex
    println!("1. Lexing...");
    let mut lexer = Lexer::new(&source);
    let tokens = lexer.tokenize().context("Lexing failed")?;
    println!("   ✓ Generated {} tokens", tokens.len());

    // Parse
    println!("2. Parsing...");
    let mut parser = QuarkParser::new(tokens);
    let ast = parser.parse().context("Parsing failed")?;
    println!("   ✓ Generated AST");

    // Visualize
    println!("3. Visualizing...");
    let mut visualizer = Visualizer::new();
    let dot_content = visualizer.visualize(&ast);

    fs::write(&dot_output, &dot_content)
        .context(format!("Failed to write DOT file: {}", dot_output.display()))?;
    println!("   ✓ Generated DOT file: {}", dot_output.display());

    generate_png(&dot_output, &png_output)?;

    println!("\n=== Compilation Complete ===");

    Ok(())
}

fn generate_png(dot_file: &PathBuf, png_file: &PathBuf) -> Result<()> {
    let output = Command::new("dot")
        .arg("-Tpng")
        .arg(dot_file)
        .arg("-o")
        .arg(png_file)
        .output();

    match output {
        Ok(output) => {
            if output.status.success() {
                println!("   ✓ Generated PNG file: {}", png_file.display());
                Ok(())
            } else {
                let error = String::from_utf8_lossy(&output.stderr);
                Err(anyhow::anyhow!("dot command failed: {}", error))
            }
        }
        Err(e) => {
            println!("   ⚠ Warning: Could not generate PNG (is graphviz installed?)");
            println!("     Error: {}", e);
            println!("     DOT file is available at: {}", dot_file.display());
            Ok(())
        }
    }
}
