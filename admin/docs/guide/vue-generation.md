# Vue Index.vue 自动生成


admin 的 Server 在注册模型时，会按 `(ModuleName, Singular)` 自动写一份 Vue 3 `<script setup>` 模板到 `WithVueOutputDir` 指向的目录——典型的 Vite `views/` 根目录（`<repo>/web/src/views`）。这样新增一个内置模型后，前端不用手写 `Index.vue` 也能跑起 CRUD 页面。

## 启用

```go
s := admin.New(
    admin.WithDB(db),
    admin.WithRouter(httpSrv),
    admin.WithVueOutputDir("admin/web/src/views"), // 与 web/ 的 Vite alias 对齐
)
```

或对单个模型（`web/` 里已有的 9 个 `sys_*` 视图是手写的，**不要**让生成器覆盖）：

```go
s.RegisterModel(&SomeNewModel{}, admin.WithRegisterVueOutputDir("admin/web/src/views"))  // 启用
s.RegisterModel(&SysUsers{},    admin.WithRegisterVueOutputDir(""))                       // 显式跳过
```

## 生成内容

每个 `Index.vue` 都是同一份模板（`vueTemplate` 常量），占位符替换：

```vue
<script setup lang="ts">
defineOptions({ name: 'SystemSysUsers' })
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { SchemaViewer } from '@sjlit/rest-ui'
const route = useRoute()
const title = computed(() => (route.meta.title as string | undefined) || 'sys_users')
</script>
<template>
  <SchemaViewer module="system" table="sys_users" :title="title" />
</template>
```

文件路径：`{outputDir}/{module-lowercase}/{singular}/Index.vue`（与 `deriveViewPath` 写进 `sys_menus.view_path` 的路径同构）。

## 行为契约

- **幂等**：已存在的文件**直接跳过**（不覆盖手写修改），所以重启 Setup 不会破坏手写的视图；
- **失败 warn 不中断**：文件系统失败（只读挂载、容器内无源码树）只 `Logger.Warn`，**不**回滚已经写入的菜单 / 权限——后者是数据库契约，前端文件是开发期便利；
- **空 ModuleName/Singular 跳过**：与菜单 / 权限的 skip 信号一致，没有派生依据就不写；
- **路径遍历保护**：`vuePathForModel` 拒绝包含 `..` 或路径分隔符的 module/singular，误信源（配置驱动的 TableName 解析器）也写不出 `Index.vue`。

测试覆盖见 `vue_gen_test.go`：`buildVueContent` / `vuePathForModel` / `generateVueFile` / `generateVueForResource`。
