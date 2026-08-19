<script setup>
import { SchemaViewer } from '@nobla/rest-ui'
</script>

<template>
    <SchemaViewer module="system" table="sys_users">
        <template #gridview="{ schema, model }"></template>
    </SchemaViewer>
</template>