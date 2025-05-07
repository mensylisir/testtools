<template>
  <div :class="{'side-menu-collapsed': isMenuCollapsed, 'side-menu': !isMenuCollapsed}">

    <div class="toggle-button" @click="toggleMenu" :title="isMenuCollapsed ? '展开菜单' : '收起菜单'">
      <i class="fa fa-bars"></i>
    </div>

    <ul v-if="!isMenuCollapsed" class="expanded-menu-list">
      <li v-for="(item, index) in MenuItems" :key="index">

        <div v-if="item.children" @click="toggleSubMenu(index)"
             :class="{'parent-active': isParentActive(item)}"
        class="menu-item-container">
          <i :class="item.icon || 'fa-solid fa-circle'" class="menu-icon"></i>
          <span class="menu-item-name"> {{ item.name }}</span>
        </div>

        <router-link v-else :to="item.path" class="menu-item-container top-level-link">
          <i :class="item.icon || 'fa-solid fa-circle'" class="menu-icon"></i>
          <span class="menu-item-name"> {{ item.name }}</span>
        </router-link>

        <ul v-if="item.children" v-show="isSubMenuOpen[index]" class="sub-menu-list">
          <li v-for="(child, childIndex) in item.children" :key="childIndex">
            <router-link :to="child.path" class="sub-menu-link">
              <i :class="child.icon || 'fa-solid fa-circle'" class="menu-icon"></i>
              {{ child.name }}
            </router-link>
          </li>
        </ul>
      </li>
    </ul>

    <ul v-if="isMenuCollapsed" class="collapsed-menu-list">
      <li v-for="(item, index) in MenuItems" :key="index">
        <div class="collapsed-menu-item" @click="handleCollapsedItemClick(item)" :title="item.name"
             :class="{'collapsed-active': isItemActive(item) || isParentActive(item)}">
          <i :class="item.icon || 'fa-solid fa-circle'" class="menu-icon"></i>
        </div>
      </li>
    </ul>
  </div>
</template>

<script>
import {ref, toRefs, onMounted} from 'vue';
import { useRouter, useRoute } from 'vue-router';

export default {
  name: 'SideMenu',
  emits: ['toggleMenu', 'CollapsedItemClick'],
  props: {
    isMenuCollapsed: {
      type: Boolean,
      default: false
    }
  },
  setup(props, {emit}) {
    const router = useRouter();
    const route = useRoute();
    const MenuItems = [
      {
        icon: 'fa-solid fa-home',
        name: '首页',
        path: '/',
      },
      {
        icon: 'fa-solid fa-tools',
        name: "测试工具",
        children: [
          {
            icon: 'fa-solid fa-server',
            path: '/dig',
            name: "DNS测试",
          },
          {
            icon: 'fa-solid fa-signal',
            path: '/ping',
            name: "连通性测试",
          },
          {
            icon: 'fa-solid fa-hdd',
            path: '/fio',
            name: "IO测试",
          }
        ]
      },
    ];
    const isSubMenuOpen = ref(Array(MenuItems.length).fill(false));


    const toggleSubMenu = (index) => {
      if (MenuItems[index] && MenuItems[index].children) {
        const currentState = isSubMenuOpen.value[index];
        isSubMenuOpen.value = Array(MenuItems.length).fill(false);
        if (!currentState) {
          isSubMenuOpen.value[index] = true;
        }
      }
    };

    const isItemActive = (item) => {
      if (item.path) {
        return route.path === item.path;
      }
      return false;
    }

    const isParentActive = (parentItem) => {
      if (!parentItem.children) return false;
      return parentItem.children.some((child) => route.path === child.path);
    }

    onMounted(()=> {
      MenuItems.forEach((item, index) => {
        if (item.children && isParentActive(item)) {
          isSubMenuOpen.value[index] = true;
        }
      });
    });

    const handleCollapsedItemClick = (item) => {
      console.log('Click item in collapsed state:', item.name);
      if (item.path) {
        console.log("Navigating to:", item.path);
        router.push(item.path);
      } else if (item.children) {
        emit("CollapsedItemClick", item);
        console.log("Parent item clicked in collapsed state. Emmitting event for popover")
      }
    };

    const toggleMenu = () => {
      const newIsMenuCollapsed = !props.isMenuCollapsed;
      emit("toggleMenu", newIsMenuCollapsed);

      if (newIsMenuCollapsed) {
        isSubMenuOpen.value = Array(MenuItems.length).fill(false);
      }
    };

    const {isMenuCollapsed} = toRefs(props);

    return {
      MenuItems,
      isMenuCollapsed,
      isSubMenuOpen,
      toggleSubMenu,
      toggleMenu,
      handleCollapsedItemClick,
      isParentActive,
      isItemActive,
    }
  },
}
</script>


<style scoped>
.side-menu {
  width: 200px;
  background: var(--background-color);;
  color: var(--text-primary);
  padding: 15px 10px;
  transition: width 0.3s ease, padding 0.3s ease;
  box-sizing: border-box;
  flex-shrink: 0;
  overflow-y: auto;
  overflow-x: hidden;
  box-shadow: 2px 0 5px rgba(0, 0, 0, 0.05);
  display: flex;
  flex-direction: column;
  z-index: 100;
}

.side-menu-collapsed {
  width: 60px;
  background: var(--background-color);
  color: var(--text-primary);
  padding: 15px 5px;
  transition: width 0.3s ease, padding 0.3s ease;
  box-sizing: border-box;
  flex-shrink: 0;
  overflow-y: auto;
  overflow-x: hidden;
  box-shadow: 2px 0 5px rgba(0, 0, 0, 0.05);
  display: flex;
  flex-direction: column;
  z-index: 100;
}

.toggle-button {
  cursor: pointer;
  margin-bottom: 5px;
  text-align: center;
  font-size: 1.4em;
  color: var(--text-secondary);
  padding: 0;
  border-radius: 2px;
  transition: color 0.2s ease, background-color 0.2s ease;
}

.side-menu .toggle-button {
  text-align: right;
}

.side-menu-collapsed .toggle-button {
  text-align: center;
}

.toggle-button:hover {
  color: var(--text-primary);
  background-color: var(--primary-color-light);
}

.expanded-menu-list,
.collapsed-menu-list {
  list-style-type: none;
  padding: 0;
  margin: 0;
  flex-grow: 1;
}

.expanded-menu-list > li,
.expanded-menu-list > li {
  margin-bottom: 6px;
}

.menu-item-container {
  cursor: pointer;
  display: flex;
  align-items: center;
  padding: 10px 8px;
  border-radius: 4px;
  transition: background-color 0.2s ease, color 0.2s ease;
  color: var(--text-primary);
  text-decoration: none;
  justify-content: flex-start;
  gap: 10px;
}

.menu-item-container:hover {
  background-color: var(--primary-color-light);
  color: var(--text-primary);
}

.menu-item-container.router-link-active,
.sub-menu-list .sub-menu-link.router-link-active {
  font-weight: bold;
  color: var(--primary-color);
  background-color: var(--primary-color-light);
}

.expanded-menu-list > li > div.parent-active {
  font-weight: bold;
  color: var(--primary-color);
  background-color: var(--primary-color-light);
}

.menu-icon {
  font-size: 1.1em;
  flex-shrink: 0;
}

.menu-item-name {
  flex-grow: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sub-menu-list {
  list-style-type: none;
  margin-top: auto;
  padding-top: 8px;
  border-left: none;
  margin-left: 0;
  padding-bottom: 4px;
  padding-left: 20px;
}

.sub-menu-list li {
  margin-bottom: 3px;
  padding-top: 8px;
}

.sub-menu-list .sub-menu-link {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 0px;
  color: var(--text-primary);
  text-decoration: none;
  transition: color 0.2s ease, background-color 0.2s ease;
  border-radius: 4px;
}

.sub-menu-list .sub-menu-link:hover {
  color: var(--text-primary);
  background-color: var(--primary-color-light);
}

.sub-menu-list .sub-menu-link.router-link-active {
  font-weight: bold;
  color: var(--primary-color);
  background-color: var(--primary-color-light);
}

.collapsed-menu-list {
  margin-top: 20px;
}

.collapsed-menu-item {
  cursor: pointer;
  display: flex;
  justify-content: center;
  align-items: center;
  width: 50px;
  height: 40px;
  margin: 0 auto 6px auto;
  border-radius: 4px;
  transition: background-color 0.2s ease, color 0.2s ease;
  font-size: 1.3em;
  color: var(--text-secondary);
}

.collapsed-menu-item:hover {
  background-color: var(--background-color);
  color: var(--text-primary);
}

.collapsed-menu-item.collapsed-active {
  color: var(--primary-color);
  background-color:var(--primary-color-light);
}

.collapsed-menu-item .menu-icon {
  margin-right: 0;
}
</style>