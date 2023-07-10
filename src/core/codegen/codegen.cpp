#include <iostream>
#include <Python.h>

#define PY_SSIZE_T_CLEAN
using namespace std;

extern "C" void consumeTree(void *obj)
{
    PyObject *treeObj, *printFunc, *result;

    treeObj = static_cast<PyObject*>(obj);
    printFunc = PyObject_GetAttrString(treeObj, "print");
    result = PyObject_CallFunction(printFunc, NULL);

    cout << "Tree Representation: \n\n"
         << PyUnicode_AsUTF8(PyObject_Str(result)) << endl;
}