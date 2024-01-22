import subprocess


class QuarkAssembler:
    def assemble(self, asm_path, out_path=""):
        self.filename = asm_path[asm_path.rfind("/") + 1 : asm_path.rfind(".")]
        result = subprocess.run(
            [
                "nasm",
                "-f",
                "win64",
                asm_path,
                "-o",
                out_path if bool(out_path) else f"build/{self.filename}.obj",
            ],
            capture_output=True,
            text=True,
        )
        print(result.stdout)
        print(result.stderr)

    def link(self, obj_path="", out_path=""):
        subprocess.run(
            [
                "gcc",
                "-o",
                out_path if bool(out_path) else f"build/{self.filename}.exe",
                obj_path if bool(obj_path) else f"build/{self.filename}.obj",
            ]
        )
